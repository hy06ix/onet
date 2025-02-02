package onet

import (
	"testing"

	"github.com/hy06ix/onet/log"
	"github.com/stretchr/testify/require"
	bbolt "go.etcd.io/bbolt"
	uuid "gopkg.in/satori/go.uuid.v1"
)

func TestServer_ProtocolRegisterName(t *testing.T) {
	c := NewLocalServer(tSuite, 0)
	defer c.Close()
	plen := len(c.protocols.instantiators)
	require.True(t, plen > 0)
	id, err := c.ProtocolRegister("ServerProtocol", NewServerProtocol)
	log.ErrFatal(err)
	require.NotNil(t, id)
	require.True(t, plen < len(c.protocols.instantiators))
	_, err = c.protocolInstantiate(ProtocolID(uuid.Nil), nil)
	require.NotNil(t, err)
	// Test for not overwriting
	_, err = c.ProtocolRegister("ServerProtocol", NewServerProtocol2)
	require.NotNil(t, err)
}

func TestServer_GetService(t *testing.T) {
	c := NewLocalServer(tSuite, 0)
	defer c.Close()
	s := c.Service("nil")
	require.Nil(t, s)
}

func TestServer_Database(t *testing.T) {
	c := NewLocalServer(tSuite, 0)
	require.NotNil(t, c.serviceManager.db)

	for _, s := range c.serviceManager.availableServices() {
		c.serviceManager.db.Update(func(tx *bbolt.Tx) error {
			b := tx.Bucket([]byte(s))
			require.NotNil(t, b)
			return nil
		})
	}
	c.Close()
}

type ServerProtocol struct {
	*TreeNodeInstance
}

// NewExampleHandlers initialises the structure for use in one round
func NewServerProtocol(n *TreeNodeInstance) (ProtocolInstance, error) {
	return &ServerProtocol{n}, nil
}

// NewExampleHandlers initialises the structure for use in one round
func NewServerProtocol2(n *TreeNodeInstance) (ProtocolInstance, error) {
	return &ServerProtocol{n}, nil
}

func (cp *ServerProtocol) Start() error {
	return nil
}
