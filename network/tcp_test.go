package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/hy06ix/onet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/kyber/v3/util/key"
	"golang.org/x/xerrors"
)

func init() {
	RegisterMessage(BigMsg{})
	SimpleMessageType = RegisterMessage(SimpleMessage{})
}

type BigMsg struct {
	Array []byte
}

type fakeConn struct {
	// how many bytes does it write at maximum at each call
	max int
	// do we fail on the first write
	fail1 bool
	done1 bool
	// do we fail on every successive write
	failRest bool
	// how many total bytes have we written
	writtenBytes int
	*net.TCPConn
}

type fakeAddr string

func (f fakeAddr) Network() string {
	return "network"
}

func (f fakeAddr) String() string {
	return "network-string"
}

func (f *fakeConn) Read(b []byte) (n int, e error) {
	return 0, nil
}

func (f *fakeConn) Write(b []byte) (n int, e error) {
	if !f.done1 && f.fail1 {
		return 0, ErrUnknown
	} else if f.failRest {
		return 0, ErrUnknown
	}
	if len(b) < f.max {
		f.writtenBytes += len(b)
		return len(b), nil
	}
	f.writtenBytes += f.max
	return f.max, nil
}

func TestTCPsendRaw(t *testing.T) {
	tests := []struct {
		msg           []byte
		conn          *fakeConn
		errExpected   bool
		bytesExpected int
	}{
		{ // fail at writing size
			make([]byte, 100),
			&fakeConn{100, true, false, false, 0, &net.TCPConn{}},
			true,
			0,
		},
		{ // fail at writing msg
			make([]byte, 100),
			&fakeConn{100, false, false, true, 0, &net.TCPConn{}},
			true,
			0,
		},
		{ // write undersize message
			make([]byte, 99),
			&fakeConn{100, false, false, false, 0, &net.TCPConn{}},
			false,
			99,
		},
		{ // write exact message
			make([]byte, 100),
			&fakeConn{100, false, false, false, 0, &net.TCPConn{}},
			false,
			100,
		},
		{ // write oversize message
			make([]byte, 101),
			&fakeConn{100, false, false, false, 0, &net.TCPConn{}},
			false,
			101,
		},
	}

	for i, test := range tests {
		tcp := &TCPConn{
			conn: test.conn,
		}
		_, err := tcp.sendRaw(test.msg)
		if test.errExpected {
			if err == nil {
				t.Error("Should have had an error here")
			}
			continue
		}
		// - 4 is for the size, uint32_t
		if test.bytesExpected != test.conn.writtenBytes-4 {
			t.Error(i, "Wrong number of bytes? ", test.bytesExpected, test.conn.writtenBytes)
		}
	}
}

// Test the receiving part of a message for tcp connections if the response is
// buffered correctly.
func TestTCPConnReceiveRaw(t *testing.T) {
	addr := make(chan string)
	done := make(chan bool)
	check := make(chan bool)

	checking := func() bool {
		select {
		case <-check:
			return false
		case <-time.After(20 * time.Millisecond):
			return true
		}
	}
	// prepare the msg
	msg := &BigMsg{Array: make([]byte, 7893)}
	buff, err := Marshal(msg)
	require.Nil(t, err)

	fn := func(c net.Conn) {
		// different slices of bytes
		maxChunk := 1400
		slices := make([][]byte, 0)
		currentChunk := 0
		for currentChunk+maxChunk < len(buff) {
			slices = append(slices, buff[currentChunk:currentChunk+maxChunk])
			currentChunk += maxChunk
		}
		slices = append(slices, buff[currentChunk:])
		// send the size first
		binary.Write(c, globalOrder, Size(len(buff)))
		// then send pieces and check if the other side already returned or not
		for i, slice := range slices[:len(slices)-1] {
			log.Lvlf1("Will write slice %d/%d...", i+1, len(slices))
			if n, err := c.Write(slice); err != nil || n != len(slice) {
				t.Fatal("Could not write enough")
			}
			log.Lvl1(" OK")
			if !checking() {
				t.Fatal("Already returned even if not finished")
			}
			time.Sleep(5 * time.Millisecond)
		}
		// the last one should make the other end return
		log.Lvl1("Will write last piece...")
		if n, err := c.Write(slices[len(slices)-1]); n != len(slices[len(slices)-1]) || err != nil {
			t.Fatal("could not send the last piece")
		}
		log.Lvl1(" OK")
		check <- true
	}

	fnBad := func(c net.Conn) {
		// send the size first
		binary.Write(c, globalOrder, Size(MaxPacketSize+1))
	}

	listen := func(f func(c net.Conn)) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.Nil(t, err)
		addr <- ln.Addr().String()
		c, err := ln.Accept()
		require.Nil(t, err)
		f(c)
		<-done
		require.Nil(t, ln.Close())
		done <- true
	}
	go listen(fn)

	// get addr
	listeningAddr := <-addr
	c, err := NewTCPConn(NewTCPAddress(listeningAddr), tSuite)
	require.Nil(t, err)

	buffRaw, err := c.receiveRaw()
	checking()
	if !bytes.Equal(buff, buffRaw) {
		t.Fatal("Bytes are not the same ")
	} else if err != nil {
		t.Error(err)
	}

	// tell the listener to close
	done <- true
	// wait until it is closed
	<-done

	go listen(fnBad)

	listeningAddr = <-addr
	c, err = NewTCPConn(NewTCPAddress(listeningAddr), tSuite)
	require.Nil(t, err)

	_, err = c.receiveRaw()
	require.NotNil(t, err)

	require.Nil(t, c.Close())
	// tell the listener to close
	done <- true
	// wait until it is closed
	<-done

}

// test the creation of a new conn by opening a golang
// listener and making a TCPConn connect to it,then close it.
func TestTCPConn(t *testing.T) {
	addr := make(chan string)
	done := make(chan bool)

	_, err := NewTCPConn(NewTCPAddress("127.0.0.1:7878"), tSuite)
	if err == nil {
		t.Fatal("Should not be able to connect here")
	}
	go func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.Nil(t, err)
		addr <- ln.Addr().String()
		_, err = ln.Accept()
		require.Nil(t, err)
		// wait until it can be closed
		<-done
		require.Nil(t, ln.Close())
		done <- true
	}()

	// get addr
	listeningAddr := <-addr
	c, err := NewTCPConn(NewTCPAddress(listeningAddr), tSuite)
	require.Nil(t, err)
	require.Equal(t, c.Local().NetworkAddress(), c.conn.LocalAddr().String())
	require.Equal(t, c.Type(), PlainTCP)
	require.Nil(t, c.Close())
	// tell the listener to close
	done <- true
	// wait until it is closed
	<-done
}

func TestTCPConnTimeout(t *testing.T) {
	var timeoutForTest = 100 * time.Millisecond

	// Need to lock because using 'timeout' from 'tcp.go'.
	timeoutLock.Lock()
	tmp := timeout
	timeout = timeoutForTest
	dialTimeout = timeoutForTest
	timeoutLock.Unlock()

	defer func() {
		timeoutLock.Lock()
		timeout = tmp
		timeoutLock.Unlock()
	}()

	addr := NewAddress(PlainTCP, "127.0.0.1:5678")
	ln, err := NewTCPListener(addr, tSuite)
	if err != nil {
		t.Fatal("error setup listener", err)
	}
	ready := make(chan bool)
	connStat := make(chan error)

	connFn := func(c Conn) {
		// receive first a good packet
		_, err := c.Receive()
		connStat <- err

		// then this receive should throw out the error
		_, err = c.Receive()
		connStat <- err

		// put the far-side receiver into broken mode
		tc := c.(*TCPConn)
		tc.receiveRawTest = func() ([]byte, error) {
			time.Sleep(2 * timeoutForTest)
			return nil, nil
		}

		// this should throw also: need to send enough bytes here
		// that we overload the kernel's buffers and it creates
		// back-pressure on us to stop sending by blocking on
		// the send system call so that Go's SendDeadline is passed.
		msg := &BigMsg{Array: make([]byte, 20*1e6)}
		_, err = c.Send(msg)
		connStat <- err
	}
	go func() {
		ready <- true
		err := ln.Listen(connFn)
		require.Nil(t, err, "Listener stop incorrectly")
	}()

	<-ready
	c, err := NewTCPConn(addr, tSuite)
	require.Nil(t, err, "Could not open connection")
	// Test bandwitdth measurements also
	sentLen, err := c.Send(&SimpleMessage{3})
	require.Nil(t, err)
	require.NotZero(t, sentLen)
	select {
	case err := <-connStat:
		assert.NoError(t, err)
	case <-time.After(2 * timeoutForTest):
		t.Error("Did not received message after timeout...")
	}

	// find the timeout from recv
	select {
	case err := <-connStat:
		assert.Error(t, err)
	case <-time.After(2 * timeoutForTest):
		t.Error("Did not received message after timeout...")
	}

	// the timeout from send too
	select {
	case err := <-connStat:
		assert.True(t, xerrors.Is(err, ErrTimeout))
	case <-time.After(10 * timeoutForTest):
		t.Error("Did not received message after timeout...")
	}

	assert.Nil(t, c.Close())
	assert.Nil(t, ln.Stop())
}

func TestTCPDialTimeout(t *testing.T) {
	oldDialTimeout := dialTimeout
	SetTCPDialTimeout(100 * time.Millisecond)
	defer SetTCPDialTimeout(oldDialTimeout)

	// We're hoping that Cloudflare is not running a server on this address
	// so we'll get a timeout error.
	addr := NewAddress(PlainTCP, "1.1.1.2:1234")
	done := make(chan struct{}, 1)
	go func() {
		_, err := NewTCPConn(addr, tSuite)
		require.Contains(t, err.Error(), "timeout")
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "should have timed out after a short duration")
	}
}

func TestTCPConnWithListener(t *testing.T) {
	addr := NewAddress(PlainTCP, "127.0.0.1:5678")
	ln, err := NewTCPListener(addr, tSuite)
	if err != nil {
		t.Fatal("error setup listener", err)
	}
	ready := make(chan bool)
	stop := make(chan bool)
	connStat := make(chan uint64)

	connFn := func(c Conn) {
		connStat <- c.Rx()
		c.Receive()
		connStat <- c.Rx()
	}
	go func() {
		ready <- true
		err := ln.Listen(connFn)
		require.Nil(t, err, "Listener stop incorrectly")
		stop <- true
	}()

	<-ready
	c, err := NewTCPConn(addr, tSuite)
	require.Nil(t, err, "Could not open connection")
	// Test bandwitdth measurements also
	rx1 := <-connStat
	tx1 := c.Tx()
	sentLen, err := c.Send(&SimpleMessage{3})
	require.Nil(t, err)
	tx2 := c.Tx()
	rx2 := <-connStat

	if (tx2 - tx1) != (rx2 - rx1) {
		t.Errorf("Connections did see same bytes? %d tx vs %d rx", (tx2 - tx1), (rx2 - rx1))
	}
	require.Equal(t, tx2-tx1, sentLen)

	require.Nil(t, ln.Stop(), "Error stopping listener")
	select {
	case <-stop:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Could not stop listener")

	}
}

// will create a TCPListener with a specific address to listen on and
// open a golang net.TCPConn to it
func TestTCPListenerWithListenAddr(t *testing.T) {
	// Testing different wrong configurations.
	testVectorWrong := []struct {
		addr   Address
		listen string
	}{
		{NewAddress(PlainTCP, "1.2.3.4:0"), ":0"},
	}
	for _, tv := range testVectorWrong {
		_, err := NewTCPListenerWithListenAddr(tv.addr, tSuite, tv.listen)
		require.NotNil(t, err, fmt.Sprintf(`Should fail: invalid combination of 
			'addr' (%s) and 'listenAddr' (%s)`, tv.addr, tv.listen))
	}

	// Testing different working configurations.
	testVector := []struct {
		addr         Address
		listen       string
		expectedHost string
	}{
		{NewAddress(PlainTCP, "1.2.3.4:0"), "127.0.0.1", "127.0.0.1"},
		{NewAddress(PlainTCP, "1.2.3.4:0"), "127.0.0.1:0", "127.0.0.1"},
		{NewAddress(PlainTCP, "1.2.3.4:0"), "", "::"},
	}
	for _, tv := range testVector {
		ln, err := NewTCPListenerWithListenAddr(tv.addr, tSuite, tv.listen)
		require.Nil(t, err, "Error setup listener")
		host, _, err := net.SplitHostPort(ln.Address().NetworkAddress())
		require.Nil(t, err, "Error splitting address of listener")
		require.Equal(t, tv.expectedHost, host)

		ready := make(chan bool)
		stop := make(chan bool)
		connReceived := make(chan bool)

		connFn := func(c Conn) {
			connReceived <- true
			c.Close()
		}
		go func() {
			ready <- true
			err := ln.Listen(connFn)
			require.Nil(t, err, "Listener stop incorrectly")
			stop <- true
		}()

		<-ready
		_, err = net.Dial("tcp", ln.Address().NetworkAddress())
		require.Nil(t, err, "Could not open connection")
		<-connReceived
		require.Nil(t, ln.Stop(), "Error stopping listener")
		select {
		case <-stop:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Could not stop listener")
		}

		require.Nil(t, ln.listen(nil))
	}

	// Testing different working configurations using static ports (but not
	// listening because we don't want to risk any error).
	testVectorStaticPort := []struct {
		addr            Address
		listen          string
		expectedAddress string
	}{
		{NewAddress(PlainTCP, "1.2.3.4:1234"), "4.3.2.1", "4.3.2.1:1234"},
		{NewAddress(PlainTCP, "1.2.3.4:1234"), "4.3.2.1:4321", "4.3.2.1:4321"},
		{NewAddress(PlainTCP, "1.2.3.4:1234"), "", ":1234"},
	}
	for _, tv := range testVectorStaticPort {
		// using directly 'getListenAddress' which is used by
		// 'NewTCPListenerWithListenAddr' to figure out the address to listen
		// to given the server address and listen address from 'private.toml'
		listenAddr, err := getListenAddress(tv.addr, tv.listen)
		require.Nil(t, err, "Error getting the listen address")
		require.Equal(t, tv.expectedAddress, listenAddr)
	}
}

// will create a TCPListener globally binding & open a golang net.TCPConn to it
func TestTCPListener(t *testing.T) {
	addr := NewAddress(PlainTCP, "127.0.0.1:0")
	ln, err := NewTCPListener(addr, tSuite)
	require.Nil(t, err, "Error setup listener")

	// Making sure the listener is globally binding
	host, _, err := net.SplitHostPort(ln.Address().NetworkAddress())
	require.Nil(t, err, "Error splitting address of listener")
	require.Equal(t, "::", host, "Listener did not bind globally when given no specific listen address")

	ready := make(chan bool)
	stop := make(chan bool)
	connReceived := make(chan bool)

	connFn := func(c Conn) {
		connReceived <- true
		c.Close()
	}
	go func() {
		ready <- true
		err := ln.Listen(connFn)
		require.Nil(t, err, "Listener stop incorrectly")
		stop <- true
	}()

	<-ready
	_, err = net.Dial("tcp", ln.Address().NetworkAddress())
	require.Nil(t, err, "Could not open connection")
	<-connReceived
	require.Nil(t, ln.Stop(), "Error stopping listener")
	select {
	case <-stop:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Could not stop listener")
	}

	require.Nil(t, ln.listen(nil))
}

func TestTCPRouter(t *testing.T) {
	wrongAddr := &ServerIdentity{Address: NewLocalAddress("127.0.0.1:2000")}
	_, err := NewTCPRouter(wrongAddr, tSuite)
	if err == nil {
		t.Fatal("Should not setup Router with local address")
	}

	addr := &ServerIdentity{Address: NewAddress(PlainTCP, "127.0.0.1:2000")}
	h1, err := NewTCPRouter(addr, tSuite)
	if err != nil {
		t.Fatal("Could not setup host")
	}
	defer h1.Stop()
	_, err = NewTCPRouter(addr, tSuite)
	if err == nil {
		t.Fatal("Should not succeed with same port")
	}
}

// Test closing and opening of Host on same address
func TestTCPHostClose(t *testing.T) {
	h1, err := NewTestTCPHost(2001)
	if err != nil {
		t.Fatal("Error setup TestTCPHost")
	}
	h2, err2 := NewTestTCPHost(2002)
	if err2 != nil {
		t.Fatal("Error setup TestTCPHost2")
	}
	go h1.Listen(acceptAndClose)
	si := NewTestServerIdentity(NewLocalAddress("127.0.0.1:7878"))
	if _, err := h2.Connect(si); err == nil {
		t.Fatal("Should not connect to dummy address or different type")
	}
	_, err = h2.Connect(NewTestServerIdentity(h1.TCPListener.Address()))
	if err != nil {
		t.Fatal("Couldn't Connect()", err)
	}

	err = h1.Stop()
	if err != nil {
		t.Fatal("Couldn't close:", err)
	}
	err = h2.Stop()
	if err != nil {
		t.Fatal("Couldn't close:", err)
	}
	log.Lvl3("Finished first connection, starting 2nd")
	h3, err3 := NewTestTCPHost(2003)
	if err3 != nil {
		t.Fatal("Could not setup host", err)
	}
	go h3.Listen(acceptAndClose)
	_, err = h2.Connect(NewTestServerIdentity(h3.TCPListener.Address()))
	if err != nil {
		t.Fatal(h2, "Couldn Connect() to", h3)
	}
	log.Lvl3("Closing h3")
	err = h3.Stop()
	if err != nil {
		// try closing the underlying connection manually and fail
		t.Fatal("Couldn't Stop()", h3)
	}
}

type dummyErr struct {
	timeout   bool
	temporary bool
}

func (d *dummyErr) Timeout() bool {
	return d.timeout
}

func (d *dummyErr) Temporary() bool {
	return d.temporary
}

func (d *dummyErr) Error() string {
	return "dummy error"
}

func TestHandleError(t *testing.T) {
	require.Equal(t, ErrClosed, handleError(xerrors.New("use of closed")))
	require.Equal(t, ErrCanceled, handleError(xerrors.New("canceled")))
	require.Equal(t, ErrEOF, handleError(xerrors.New("EOF")))

	require.Equal(t, ErrUnknown, handleError(xerrors.New("random error")))

	de := dummyErr{true, true}
	de.temporary = false
	require.Equal(t, ErrTimeout, handleError(&de))
	de.timeout = false
	require.Equal(t, ErrUnknown, handleError(&de))
}

func NewTestTCPHost(port int) (*TCPHost, error) {
	addr := NewTCPAddress("127.0.0.1:" + strconv.Itoa(port))
	kp := key.NewKeyPair(tSuite)
	e := NewServerIdentity(kp.Public, addr)
	e.SetPrivate(kp.Private)
	return NewTCPHost(e, tSuite)
}

// Returns a ServerIdentity out of the address
func NewTestServerIdentity(address Address) *ServerIdentity {
	kp := key.NewKeyPair(tSuite)
	e := NewServerIdentity(kp.Public, address)
	return e
}

// SimpleMessage is just used to transfer one integer
type SimpleMessage struct {
	I int64
}

var SimpleMessageType MessageTypeID

type simpleMessageProc struct {
	t     *testing.T
	relay chan SimpleMessage
}

func newSimpleMessageProc(t *testing.T) *simpleMessageProc {
	return &simpleMessageProc{
		t:     t,
		relay: make(chan SimpleMessage),
	}
}

func (smp *simpleMessageProc) Process(e *Envelope) {
	if e.MsgType != SimpleMessageType {
		smp.t.Fatal("Wrong message")
	}
	sm := e.Msg.(*SimpleMessage)
	smp.relay <- *sm
}

type statusMessage struct {
	Ok  bool
	Val int64
}

var statusMsgID = RegisterMessage(statusMessage{})

type simpleProcessor struct {
	relay chan statusMessage
}

func newSimpleProcessor() *simpleProcessor {
	return &simpleProcessor{
		relay: make(chan statusMessage),
	}
}
func (sp *simpleProcessor) Process(env *Envelope) {
	if env.MsgType != statusMsgID {

		sp.relay <- statusMessage{false, 0}
	}
	sm := env.Msg.(*statusMessage)

	sp.relay <- *sm
}

func sendrcvProc(from, to *Router) error {
	sp := newSimpleProcessor()
	// new processing
	to.RegisterProcessor(sp, statusMsgID)
	sentLen, err := from.Send(to.ServerIdentity, &statusMessage{true, 10})
	if err != nil {
		return err
	}
	if sentLen == 0 {
		return xerrors.New("sentLen is zero")
	}

	select {
	case <-sp.relay:
		err = nil
	case <-time.After(1 * time.Second):
		err = xerrors.New("timeout")
	}
	// delete the processing
	to.RegisterProcessor(nil, statusMsgID)
	return err
}

func waitConnections(r *Router, sid *ServerIdentity) error {
	for i := 0; i < 10; i++ {
		c := r.connection(sid.ID)
		if c != nil {
			return nil
		}
		time.Sleep(WaitRetry)
	}
	return xerrors.Errorf("Didn't see connection to %s in router", sid.Address)
}

func acceptAndClose(c Conn) {
	c.Close()
	return
}
