package main

import (
	"strconv"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hy06ix/onet"
	"github.com/hy06ix/onet/log"
	"github.com/hy06ix/onet/network"
	"github.com/hy06ix/onet/simul"
	"github.com/hy06ix/onet/simul/manage"
	"github.com/hy06ix/onet/simul/monitor"
	"golang.org/x/xerrors"
)

type testInit struct{}

var testInitID network.MessageTypeID

/*
Defines the simulation for the count-protocol
*/

func init() {
	onet.SimulationRegister("CountTest", NewSimulation)

	testInitID = network.RegisterMessage(&testInit{})
}

// Simulation only holds the BFTree simulation
type simulation struct {
	onet.SimulationBFTree
	Other  string
	Ration float64
}

// NewSimulation returns the new simulation, where all fields are
// initialised using the config-file
func NewSimulation(config string) (onet.Simulation, error) {
	es := &simulation{}
	_, err := toml.Decode(config, es)
	if err != nil {
		return nil, xerrors.Errorf("decoding toml: %v", err)
	}
	return es, nil
}

// Setup creates the tree used for that simulation
func (e *simulation) Setup(dir string, hosts []string) (
	*onet.SimulationConfig, error) {
	sc := &onet.SimulationConfig{}
	e.CreateRoster(sc, hosts, 2000)
	err := e.CreateTree(sc)
	if err != nil {
		return nil, xerrors.Errorf("creating tree: %v", err)
	}
	return sc, nil
}

func (e *simulation) Node(config *onet.SimulationConfig) error {
	wg := sync.WaitGroup{}
	wg.Add(1)
	config.Server.RegisterProcessorFunc(testInitID, func(*network.Envelope) error {
		wg.Done()
		return nil
	})

	if config.Server.ServerIdentity.Equal(config.Tree.Root.ServerIdentity) {
		time.Sleep(1 * time.Second)
		for _, tn := range config.Tree.List() {
			config.Server.Send(tn.ServerIdentity, &testInit{})
		}
	}

	wg.Wait()
	return nil
}

// Run is used on the destination machines and runs a number of
// rounds
func (e *simulation) Run(config *onet.SimulationConfig) error {
	size := config.Tree.Size()
	log.Lvl2("Size is:", size, "rounds:", e.Rounds)
	for round := 0; round < e.Rounds; round++ {
		log.Lvl1("Starting round", round)
		round := monitor.NewTimeMeasure("round")
		p, err := config.Overlay.CreateProtocol("Count", config.Tree, onet.NilServiceID)
		if err != nil {
			return xerrors.Errorf("creating protocol: %v", err)
		}
		go p.Start()
		children := <-p.(*manage.ProtocolCount).Count
		round.Record()
		if children != size {
			return xerrors.New("Didn't get " + strconv.Itoa(size) +
				" children")
		}
	}
	return nil
}

func main() {
	simul.Start()
}
