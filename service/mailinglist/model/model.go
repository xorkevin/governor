package model

//go:generate forge model -m ListModel -t mailinglists -p list -o modellist_gen.go ListModel
//go:generate forge model -m MemberModel -t mailinglistmembers -p member -o modelmember_gen.go MemberModel
//go:generate forge model -m MsgModel -t mailinglistmsgs -p msg -o modelmsg_gen.go MsgModel

type (
	// ListModel is the db mailing list model
	ListModel struct {
		ListID       string `model:"listid,VARCHAR(255) PRIMARY KEY" query:"listid;getoneeq,listid;getgroupeq,listid|arr;updeq,listid;deleq,listid"`
		CreatorID    string `model:"creatorid,VARCHAR(31) NOT NULL" query:"creatorid;deleq,creatorid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Description  string `model:"description,VARCHAR(255)" query:"description"`
		LastUpdated  int64  `model:"last_updated,BIGINT NOT NULL;index,creatorid" query:"last_updated;getgroupeq,creatorid;getgroupeq,creatorid,last_updated|lt"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	// MemberModel is the db mailing list member model
	MemberModel struct {
		ListID      string `model:"listid,VARCHAR(255)" query:"listid;deleq,listid"`
		Userid      string `model:"userid,VARCHAR(31), PRIMARY KEY (listid, userid)" query:"userid;getgroupeq,listid;deleq,listid,userid|arr;deleq,userid"`
		LastUpdated int64  `model:"last_updated,BIGINT NOT NULL;index,userid" query:"last_updated;getgroupeq,userid;getgroupeq,userid,last_updated|lt"`
	}

	// MsgModel is the db mailing list message model
	MsgModel struct {
		ListID       string `model:"listid,VARCHAR(255)" query:"listid;deleq,listid"`
		Msgid        string `model:"msgid,VARCHAR(1023), PRIMARY KEY (listid, msgid)" query:"msgid;deleq,listid,msgid|arr"`
		Userid       string `model:"userid,VARCHAR(31) NOT NULL" query:"userid"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL;index,listid" query:"creation_time;getgroupeq,listid;getgroupeq,listid,creation_time|lt"`
	}
)
