package state

type (
	Model struct {
		Orgname      string
		Setup        bool
		CreationTime int64
	}

	ReqSetup struct {
		Orgname string
	}

	State interface {
		Get() (*Model, error)
		Set(m *Model) error
		Setup(req ReqSetup) error
	}
)
