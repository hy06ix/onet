package network

import (
	"sync"

	"github.com/hy06ix/onet/log"
	"golang.org/x/xerrors"
)

// Dispatcher is an interface whose sole role is to distribute messages to the
// right Processor. No processing is done,i.e. no looking at packet content.
// Each Processor that wants to receive all messages of a specific
// type must register itself to the dispatcher using `RegisterProcessor()`.
// The network layer calls `Dispatch()` each time it receives a message, so
// the dispatcher is able to dispatch correctly to the corresponding Processor.
// Two Dispatchers are available:
//   * BlockingDispatcher - waits for the return of the Processor before taking
//     another message
//   * RoutineDispatcher - starts every Processor in a go-routine
type Dispatcher interface {
	// RegisterProcessor is called by a Processor so it can receive all messages
	// of type msgType. If given multiple msgType, the same processor will be
	// called for each of the msgType given.
	// **NOTE** In the current version, if a subsequent call to RegisterProcessor
	// happens for the same msgType, the latest Processor will be used; there
	// is no *copy* or *duplication* of messages.
	RegisterProcessor(p Processor, msgType ...MessageTypeID)
	// RegisterProcessorFunc enables to register directly a function that will
	// be called for each message of type msgType. It's a shorter way of
	// registering a Processor.
	RegisterProcessorFunc(MessageTypeID, func(*Envelope) error)
	// Dispatch will find the right processor to dispatch the packet to. The id
	// is the identity of the author / sender of the packet.
	// It can be called for example by the network layer.
	// If no processor is found for this message type, then an error is returned
	Dispatch(*Envelope) error
}

// Processor is an abstraction to represent any object that want to process
// messages. It is used in conjunction with Dispatcher:
// A processor must register itself to a Dispatcher so the Dispatcher will
// dispatch every messages asked for to the Processor.
type Processor interface {
	// Process takes a received Envelope.
	Process(*Envelope)
}

// BlockingDispatcher is a Dispatcher that simply calls `p.Process()` on a
// processor p each time it receives a message with `Dispatch`. It does *not*
// launch a go routine, or put the message in a queue, etc.
// It can be re-used for more complex dispatchers.
type BlockingDispatcher struct {
	sync.Mutex
	procs map[MessageTypeID]Processor
}

// NewBlockingDispatcher will return a new BlockingDispatcher.
func NewBlockingDispatcher() *BlockingDispatcher {
	return &BlockingDispatcher{
		procs: make(map[MessageTypeID]Processor),
	}
}

// RegisterProcessor saves the given processor in the dispatcher.
func (d *BlockingDispatcher) RegisterProcessor(p Processor, msgType ...MessageTypeID) {
	d.Lock()
	defer d.Unlock()
	for _, t := range msgType {
		d.procs[t] = p
	}
}

// RegisterProcessorFunc takes a func, creates a Processor struct around it and
// registers it to the dispatcher.
func (d *BlockingDispatcher) RegisterProcessorFunc(msgType MessageTypeID, fn func(*Envelope) error) {
	p := &defaultProcessor{
		fn: fn,
	}
	d.RegisterProcessor(p, msgType)
}

// Dispatch calls the corresponding processor's method Process. It's a
// blocking call if the Processor is blocking.
func (d *BlockingDispatcher) Dispatch(packet *Envelope) error {
	defer log.Lvl5("done tracing")
	// cannot use the "defer unlock" idiom here because we cannot
	// be holding the lock while calling p.Process in case the
	// processor wants to call RegisterProcessor.
	d.Lock()
	var p Processor
	if p = d.procs[packet.MsgType]; p == nil {
		d.Unlock()
		return xerrors.New("No Processor attached to this message type " + packet.MsgType.String())
	}
	d.Unlock()
	p.Process(packet)
	return nil
}

// RoutineDispatcher dispatches messages to the Processors
// in a go routine. RoutineDispatcher creates one go routine per messages it
// receives.
type RoutineDispatcher struct {
	*BlockingDispatcher
	// routines counts how many routines are running
	routines      int
	routinesMutex sync.Mutex
}

// NewRoutineDispatcher returns a fresh RoutineDispatcher
func NewRoutineDispatcher() *RoutineDispatcher {
	return &RoutineDispatcher{
		BlockingDispatcher: NewBlockingDispatcher(),
	}
}

// Dispatch implements the Dispatcher interface. It will give the packet to the
// right Processor in a go routine.
func (d *RoutineDispatcher) Dispatch(packet *Envelope) error {
	defer log.Lvl5("done tracing")
	d.Lock()
	defer d.Unlock()
	var p = d.procs[packet.MsgType]
	if p == nil {
		return xerrors.New("no Processor attached to this message type")
	}
	go func() {
		d.routinesMutex.Lock()
		d.routines++
		d.routinesMutex.Unlock()
		p.Process(packet)
		d.routinesMutex.Lock()
		d.routines--
		d.routinesMutex.Unlock()
	}()
	return nil
}

// GetRoutines returns how many routines are waiting.
func (d *RoutineDispatcher) GetRoutines() int {
	d.routinesMutex.Lock()
	defer d.routinesMutex.Unlock()
	return d.routines
}

type defaultProcessor struct {
	fn func(*Envelope) error
}

func (dp *defaultProcessor) Process(msg *Envelope) {
	err := dp.fn(msg)
	if err != nil {
		log.Errorf("error while processing msg type %v from %v: %+v", msg.MsgType, msg.ServerIdentity, err)
	}
}
