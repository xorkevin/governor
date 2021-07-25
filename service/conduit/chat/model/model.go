package model

import (
	"xorkevin.dev/governor/service/db"
)

type (
	Repo interface {
		Setup() error
	}

	repo struct {
		db db.Database
	}

	// MsgModel is the db message model
	MsgModel struct {
		Chatid string `model:"chatid,VARCHAR(31)" query:"chatid"`
		Msgid  string `model:"msgid,VARCHAR(31), PRIMARY KEY (chatid, msgid);index,chatid,kind" query:"msgid;getoneeq,chatid,msgid;getgroupeq,chatid;getgroupeq,chatid,kind;deleq,chatid,msgid;deleq,chatid"`
		Userid string `model:"userid,VARCHAR(31) NOT NULL" query:"userid"`
		Timems int64  `model:"time_ms,BIGINT NOT NULL" query:"time_ms"`
		Kind   int    `model:"kind,INTEGER NOT NULL" query:"kind"`
		Value  string `model:"value,VARCHAR(4095) NOT NULL" query:"value"`
	}
)
