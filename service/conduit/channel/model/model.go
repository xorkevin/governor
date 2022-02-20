package model

import (
	"xorkevin.dev/governor/service/db"
)

//go:generate forge model -m Model -p server -o model_gen.go Model
//go:generate forge model -m ChannelModel -p channel -o modelchannel_gen.go ChannelModel
//go:generate forge model -m PresenceModel -p presence -o modelpresence_gen.go PresenceModel

const (
	chatUIDSize = 16
)

type (
	Repo interface {
		Setup() error
	}

	repo struct {
		table         string
		tableChannels string
		tablePresence string
		db            db.Database
	}

	// Model is the db conduit server model
	Model struct {
		ServerID     string `model:"serverid,VARCHAR(31) PRIMARY KEY" query:"serverid;getoneeq,serverid;getgroupeq,serverid|arr;updeq,serverid;deleq,serverid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Theme        string `model:"theme,VARCHAR(4095) NOT NULL" query:"theme"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	// ChannelModel is the db server channel model
	ChannelModel struct {
		ServerID  string `model:"serverid,VARCHAR(31)" query:"serverid;deleq,serverid"`
		ChannelID string `model:"channelid,VARCHAR(31), PRIMARY KEY (serverid, channelid)" query:"channelid;getgroupeq,serverid;getgroupeq,serverid,channelid|like;updeq,serverid,channelid;deleq,serverid,channelid|arr"`
		Chatid    string `model:"chatid,VARCHAR(31) UNIQUE" query:"chatid"`
		Name      string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Desc      string `model:"desc,VARCHAR(255)" query:"desc"`
		Theme     string `model:"theme,VARCHAR(4095) NOT NULL" query:"theme"`
	}

	// PresenceModel is the db presence model
	PresenceModel struct {
		ServerID    string `model:"serverid,VARCHAR(31)" query:"serverid;deleq,serverid"`
		Userid      string `model:"userid,VARCHAR(31), PRIMARY KEY (serverid, userid)" query:"userid;updeq,serverid,userid"`
		LastUpdated int64  `model:"last_updated,BIGINT NOT NULL;index,serverid" query:"last_updated;getgroupeq,serverid,last_updated|gt;deleq,serverid,last_updated|leq"`
	}

	ctxKeyRepo struct{}
)
