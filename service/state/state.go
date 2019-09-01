package state

type (
	// Model is the data about a governor server in between restarts
	//
	// Orgname is the name of the governor server org
	// Setup is true if setup has already been run
	// CreationTime is the Unix time of when setup had been first run
	Model struct {
		Orgname      string
		Setup        bool
		CreationTime int64
	}

	// ReqSetup are the options necessary to setup the server state
	ReqSetup struct {
		Orgname string
	}

	// State is the interface for a service that records governor server state
	//
	// Get retrieves the current state
	// Set sets the state
	// Setup sets up the state
	State interface {
		Get() (*Model, error)
		Set(m *Model) error
		Setup(req ReqSetup) error
	}
)
