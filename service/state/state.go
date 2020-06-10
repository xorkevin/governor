package state

type (
	// Model is the data about a governor server in between restarts
	//
	// Setup is true if setup has already been run
	// CreationTime is the Unix time of when setup had been first run
	Model struct {
		Setup        bool
		Version      string
		VHash        string
		CreationTime int64
	}

	// ReqSetup are the options necessary to setup the server state
	ReqSetup struct {
		Version string
		VHash   string
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
