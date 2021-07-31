package model

import (
	"xorkevin.dev/governor/service/db"
)

//go:generate forge model -m ChatModel -t chats -p chat -o modelchat_gen.go ChatModel
//go:generate forge model -m MemberModel -t chatmembers -p member -o modelmember_gen.go MemberModel
//go:generate forge model -m MsgModel -t chatmessages -p msg -o modelmsg_gen.go MsgModel

type (
	Repo interface {
		Setup() error
	}

	repo struct {
		db db.Database
	}

	// ChatModel is the db chat model
	ChatModel struct {
		Chatid       string `model:"chatid,VARCHAR(31) PRIMARY KEY" query:"chatid;updeq,chatid;deleq,chatid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Theme        string `model:"theme,VARCHAR(4095) NOT NULL" query:"theme"`
		LastUpdated  int64  `model:"last_updated,BIGINT NOT NULL" query:"last_updated"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	// MemberModel is the db chat member model
	MemberModel struct {
		Chatid string `model:"chatid,VARCHAR(31);index,userid" query:"chatid"`
		Userid string `model:"userid,VARCHAR(31), PRIMARY KEY (chatid, userid)" query:"userid;getgroupeq,chatid;deleq,chatid,userid"`
	}

	// MsgModel is the db message model
	MsgModel struct {
		Chatid string `model:"chatid,VARCHAR(31)" query:"chatid"`
		Msgid  string `model:"msgid,VARCHAR(31), PRIMARY KEY (chatid, msgid);index,chatid,kind" query:"msgid;getgroupeq,chatid;getgroupeq,chatid,msgid|lt;getgroupeq,chatid,kind;getgroupeq,chatid,kind,msgid|lt;deleq,chatid,msgid;deleq,chatid"`
		Userid string `model:"userid,VARCHAR(31) NOT NULL" query:"userid"`
		Timems int64  `model:"time_ms,BIGINT NOT NULL" query:"time_ms"`
		Kind   int    `model:"kind,INTEGER NOT NULL" query:"kind"`
		Value  string `model:"value,VARCHAR(4095) NOT NULL" query:"value"`
	}
)
