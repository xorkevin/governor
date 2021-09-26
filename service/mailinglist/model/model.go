package model

type (
	// ListModel is the db mailing list model
	ListModel struct {
		CreatorID string `model:"creatorid,VARCHAR(31)"`
		ListID    string `model:"listid,VARCHAR(255)"`
	}
)
