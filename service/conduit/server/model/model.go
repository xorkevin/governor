package model

import (
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
)

//go:generate forge model -m Model -p server -o model_gen.go Model
//go:generate forge model -m ChannelModel -p channel -o modelchannel_gen.go ChannelModel
//go:generate forge model -m PresenceModel -p presence -o modelpresence_gen.go PresenceModel

const (
	chatUIDSize = 16
)

type (
	Repo interface {
		New(serverid string, name, desc string, theme string) *Model
		GetServer(serverid string) (*Model, error)
		GetChannels(serverid string, prefix string, limit, offset int) ([]ChannelModel, error)
		GetPresence(serverid string, after int64, limit, offset int) ([]PresenceModel, error)
		Insert(m *Model) error
		Update(m *Model) error
		NewChannel(serverid, channelid string, name, desc string, theme string) (*ChannelModel, error)
		InsertChannel(m *ChannelModel) error
		UpdateChannel(m *ChannelModel) error
		DeleteChannels(serverid string, channelids []string) error
		UpdatePresence(serverid string, userid string, t int64) error
		DeletePresence(serverid string, before int64) error
		Delete(serverid string) error
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
		ServerID     string `model:"serverid,VARCHAR(31) PRIMARY KEY" query:"serverid;getoneeq,serverid;updeq,serverid;deleq,serverid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Desc         string `model:"desc,VARCHAR(255)" query:"desc"`
		Theme        string `model:"theme,VARCHAR(4095) NOT NULL" query:"theme"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	// ChannelModel is the db server channel model
	ChannelModel struct {
		ServerID     string `model:"serverid,VARCHAR(31)" query:"serverid;deleq,serverid"`
		ChannelID    string `model:"channelid,VARCHAR(31), PRIMARY KEY (serverid, channelid)" query:"channelid;getgroupeq,serverid;getgroupeq,serverid,channelid|like;updeq,serverid,channelid;deleq,serverid,channelid|arr"`
		Chatid       string `model:"chatid,VARCHAR(31) UNIQUE" query:"chatid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Desc         string `model:"desc,VARCHAR(255)" query:"desc"`
		Theme        string `model:"theme,VARCHAR(4095) NOT NULL" query:"theme"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	// PresenceModel is the db presence model
	PresenceModel struct {
		ServerID    string `model:"serverid,VARCHAR(31)" query:"serverid;deleq,serverid"`
		Userid      string `model:"userid,VARCHAR(31), PRIMARY KEY (serverid, userid)" query:"userid"`
		LastUpdated int64  `model:"last_updated,BIGINT NOT NULL;index,serverid" query:"last_updated;getgroupeq,serverid,last_updated|gt;deleq,serverid,last_updated|leq"`
	}

	ctxKeyRepo struct{}
)

// GetCtxRepo returns a Repo from the context
func GetCtxRepo(inj governor.Injector) Repo {
	v := inj.Get(ctxKeyRepo{})
	if v == nil {
		return nil
	}
	return v.(Repo)
}

// SetCtxRepo sets a Repo in the context
func SetCtxRepo(inj governor.Injector, r Repo) {
	inj.Set(ctxKeyRepo{}, r)
}

func NewInCtx(inj governor.Injector, table, tableChannels, tablePresence string) {
	SetCtxRepo(inj, NewCtx(inj, table, tableChannels, tablePresence))
}

func NewCtx(inj governor.Injector, table, tableChannels, tablePresence string) Repo {
	dbService := db.GetCtxDB(inj)
	return New(dbService, table, tableChannels, tablePresence)
}

func New(database db.Database, table, tableChannels, tablePresence string) Repo {
	return &repo{
		table:         table,
		tableChannels: tablePresence,
		tablePresence: tablePresence,
		db:            database,
	}
}

// New creates new conduit server
func (r *repo) New(serverid string, name, desc string, theme string) *Model {
	return &Model{
		ServerID:     serverid,
		Name:         name,
		Desc:         desc,
		Theme:        theme,
		CreationTime: time.Now().Round(0).Unix(),
	}
}

// GetServer returns a server by id
func (r *repo) GetServer(serverid string) (*Model, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := serverModelGetModelEqServerID(d, r.table, serverid)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get server")
	}
	return m, nil
}

func (r *repo) GetChannels(serverid string, prefix string, limit, offset int) ([]ChannelModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		m, err := channelModelGetChannelModelEqServerIDOrdChannelID(d, r.tableChannels, serverid, true, limit, offset)
		if err != nil {
			return nil, db.WrapErr(err, "Failed to get channels")
		}
		return m, nil
	}
	m, err := channelModelGetChannelModelEqServerIDLikeChannelIDOrdChannelID(d, r.tableChannels, serverid, prefix+"%", true, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get channels")
	}
	return m, nil
}

func (r *repo) GetPresence(serverid string, after int64, limit, offset int) ([]PresenceModel, error) {
	d, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	m, err := presenceModelGetPresenceModelEqServerIDGtLastUpdatedOrdLastUpdated(d, r.tablePresence, serverid, after, true, limit, offset)
	if err != nil {
		return nil, db.WrapErr(err, "Failed to get presence")
	}
	return m, nil
}

func (r *repo) Insert(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := serverModelInsert(d, r.table, m); err != nil {
		return db.WrapErr(err, "Failed to insert server")
	}
	return nil
}

func (r *repo) Update(m *Model) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := serverModelUpdModelEqServerID(d, r.table, m, m.ServerID); err != nil {
		return db.WrapErr(err, "Failed to update server")
	}
	return nil
}

func (r *repo) NewChannel(serverid, channelid string, name, desc string, theme string) (*ChannelModel, error) {
	u, err := uid.New(chatUIDSize)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new uid")
	}
	return &ChannelModel{
		ServerID:     serverid,
		ChannelID:    channelid,
		Chatid:       u.Base64(),
		Name:         name,
		Desc:         desc,
		Theme:        theme,
		CreationTime: time.Now().Round(0).Unix(),
	}, nil
}

func (r *repo) InsertChannel(m *ChannelModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := channelModelInsert(d, r.tableChannels, m); err != nil {
		return db.WrapErr(err, "Failed to insert channel")
	}
	return nil
}

func (r *repo) UpdateChannel(m *ChannelModel) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := channelModelUpdChannelModelEqServerIDEqChannelID(d, r.tableChannels, m, m.ServerID, m.ChannelID); err != nil {
		return db.WrapErr(err, "Failed to update channel")
	}
	return nil
}

func (r *repo) DeleteChannels(serverid string, channelids []string) error {
	if len(channelids) == 0 {
		return nil
	}

	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := channelModelDelEqServerIDHasChannelID(d, r.tableChannels, serverid, channelids); err != nil {
		return db.WrapErr(err, "Failed to delete channels")
	}
	return nil
}

func (r *repo) UpdatePresence(serverid string, userid string, t int64) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if _, err := d.Exec("INSERT INTO "+r.tablePresence+" (serverid, userid, last_updated) VALUES ($1, $2, $3) ON CONFLICT (serverid, userid) DO UPDATE SET last_updated = EXCLUDED.last_updated;", serverid, userid, t); err != nil {
		return db.WrapErr(err, "Failed to update presence")
	}
	return nil
}

func (r *repo) DeletePresence(serverid string, before int64) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := presenceModelDelEqServerIDLeqLastUpdated(d, r.tablePresence, serverid, before); err != nil {
		return db.WrapErr(err, "Failed to delete presence")
	}
	return nil
}

func (r *repo) Delete(serverid string) error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := presenceModelDelEqServerID(d, r.tablePresence, serverid); err != nil {
		return db.WrapErr(err, "Failed to delete presence")
	}
	if err := channelModelDelEqServerID(d, r.tableChannels, serverid); err != nil {
		return db.WrapErr(err, "Failed to delete channels")
	}
	if err := serverModelDelEqServerID(d, r.table, serverid); err != nil {
		return db.WrapErr(err, "Failed to delete server")
	}
	return nil
}

// Setup creates new server, channel, and presence tables
func (r *repo) Setup() error {
	d, err := r.db.DB()
	if err != nil {
		return err
	}
	if err := serverModelSetup(d, r.table); err != nil {
		err = db.WrapErr(err, "Failed to setup server model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	if err := channelModelSetup(d, r.tableChannels); err != nil {
		err = db.WrapErr(err, "Failed to setup server channel model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	if err := presenceModelSetup(d, r.tablePresence); err != nil {
		err = db.WrapErr(err, "Failed to setup server presence model")
		if !errors.Is(err, db.ErrAuthz{}) {
			return err
		}
	}
	return nil
}
