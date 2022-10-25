package model

import (
	"context"
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/kerrors"
)

//go:generate forge model

const (
	chatUIDSize = 16
)

type (
	Repo interface {
		New(serverid string, name, desc string, theme string) *Model
		GetServer(ctx context.Context, serverid string) (*Model, error)
		GetChannel(ctx context.Context, serverid, channelid string) (*ChannelModel, error)
		GetChannels(ctx context.Context, serverid string, prefix string, limit, offset int) ([]ChannelModel, error)
		GetPresence(ctx context.Context, serverid string, after int64, limit, offset int) ([]PresenceModel, error)
		Insert(ctx context.Context, m *Model) error
		UpdateProps(ctx context.Context, m *Model) error
		NewChannel(serverid, channelid string, name, desc string, theme string) (*ChannelModel, error)
		InsertChannel(ctx context.Context, m *ChannelModel) error
		UpdateChannelProps(ctx context.Context, m *ChannelModel) error
		DeleteChannels(ctx context.Context, serverid string, channelids []string) error
		UpdatePresence(ctx context.Context, serverid string, userid string, t int64) error
		DeletePresence(ctx context.Context, serverid string, before int64) error
		Delete(ctx context.Context, serverid string) error
		Setup(ctx context.Context) error
	}

	repo struct {
		table         *serverModelTable
		tableChannels *channelModelTable
		tablePresence *presenceModelTable
		db            db.Database
	}

	// Model is the db conduit server model
	//forge:model server
	//forge:model:query server
	Model struct {
		ServerID     string `model:"serverid,VARCHAR(31) PRIMARY KEY" query:"serverid;getoneeq,serverid;deleq,serverid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Desc         string `model:"desc,VARCHAR(255)" query:"desc"`
		Theme        string `model:"theme,VARCHAR(4095) NOT NULL" query:"theme"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	//forge:model:query server
	serverProps struct {
		Name  string `query:"name;updeq,serverid"`
		Desc  string `query:"desc"`
		Theme string `query:"theme"`
	}

	// ChannelModel is the db server channel model
	//forge:model channel
	//forge:model:query channel
	ChannelModel struct {
		ServerID     string `model:"serverid,VARCHAR(31)" query:"serverid;deleq,serverid"`
		ChannelID    string `model:"channelid,VARCHAR(31), PRIMARY KEY (serverid, channelid)" query:"channelid;getoneeq,serverid,channelid;getgroupeq,serverid;getgroupeq,serverid,channelid|like;deleq,serverid,channelid|in"`
		Chatid       string `model:"chatid,VARCHAR(31) UNIQUE" query:"chatid"`
		Name         string `model:"name,VARCHAR(255) NOT NULL" query:"name"`
		Desc         string `model:"desc,VARCHAR(255)" query:"desc"`
		Theme        string `model:"theme,VARCHAR(4095) NOT NULL" query:"theme"`
		CreationTime int64  `model:"creation_time,BIGINT NOT NULL" query:"creation_time"`
	}

	//forge:model:query channel
	channelProps struct {
		Name  string `query:"name;updeq,serverid,channelid"`
		Desc  string `query:"desc"`
		Theme string `query:"theme"`
	}

	// PresenceModel is the db presence model
	//forge:model presence
	//forge:model:query presence
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
		table: &serverModelTable{
			TableName: table,
		},
		tableChannels: &channelModelTable{
			TableName: tablePresence,
		},
		tablePresence: &presenceModelTable{
			TableName: tablePresence,
		},
		db: database,
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
func (r *repo) GetServer(ctx context.Context, serverid string) (*Model, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.table.GetModelEqServerID(ctx, d, serverid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get server")
	}
	return m, nil
}

func (r *repo) GetChannel(ctx context.Context, serverid string, channelid string) (*ChannelModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tableChannels.GetChannelModelEqServerIDEqChannelID(ctx, d, serverid, channelid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get channel")
	}
	return m, nil
}

func (r *repo) GetChannels(ctx context.Context, serverid string, prefix string, limit, offset int) ([]ChannelModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		m, err := r.tableChannels.GetChannelModelEqServerIDOrdChannelID(ctx, d, serverid, true, limit, offset)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to get channels")
		}
		return m, nil
	}
	m, err := r.tableChannels.GetChannelModelEqServerIDLikeChannelIDOrdChannelID(ctx, d, serverid, prefix+"%", true, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get channels")
	}
	return m, nil
}

func (r *repo) GetPresence(ctx context.Context, serverid string, after int64, limit, offset int) ([]PresenceModel, error) {
	d, err := r.db.DB(ctx)
	if err != nil {
		return nil, err
	}
	m, err := r.tablePresence.GetPresenceModelEqServerIDGtLastUpdatedOrdLastUpdated(ctx, d, serverid, after, true, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get presence")
	}
	return m, nil
}

func (r *repo) Insert(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert server")
	}
	return nil
}

func (r *repo) UpdateProps(ctx context.Context, m *Model) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.UpdserverPropsEqServerID(ctx, d, &serverProps{
		Name:  m.Name,
		Desc:  m.Desc,
		Theme: m.Theme,
	}, m.ServerID); err != nil {
		return kerrors.WithMsg(err, "Failed to update server")
	}
	return nil
}

func (r *repo) NewChannel(serverid, channelid string, name, desc string, theme string) (*ChannelModel, error) {
	u, err := uid.New(chatUIDSize)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create new uid")
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

func (r *repo) InsertChannel(ctx context.Context, m *ChannelModel) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableChannels.Insert(ctx, d, m); err != nil {
		return kerrors.WithMsg(err, "Failed to insert channel")
	}
	return nil
}

func (r *repo) UpdateChannelProps(ctx context.Context, m *ChannelModel) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableChannels.UpdchannelPropsEqServerIDEqChannelID(ctx, d, &channelProps{
		Name:  m.Name,
		Desc:  m.Desc,
		Theme: m.Theme,
	}, m.ServerID, m.ChannelID); err != nil {
		return kerrors.WithMsg(err, "Failed to update channel")
	}
	return nil
}

func (r *repo) DeleteChannels(ctx context.Context, serverid string, channelids []string) error {
	if len(channelids) == 0 {
		return nil
	}

	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tableChannels.DelEqServerIDHasChannelID(ctx, d, serverid, channelids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete channels")
	}
	return nil
}

func (t *presenceModelTable) UpsertPresence(ctx context.Context, d db.SQLExecutor, serverid string, userid string, ti int64) error {
	if _, err := d.ExecContext(ctx, "INSERT INTO "+t.TableName+" (serverid, userid, last_updated) VALUES ($1, $2, $3) ON CONFLICT (serverid, userid) DO UPDATE SET last_updated = EXCLUDED.last_updated;", serverid, userid, t); err != nil {
		return err
	}
	return nil
}

func (r *repo) UpdatePresence(ctx context.Context, serverid string, userid string, t int64) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tablePresence.UpsertPresence(ctx, d, serverid, userid, t); err != nil {
		return kerrors.WithMsg(err, "Failed to update presence")
	}
	return nil
}

func (r *repo) DeletePresence(ctx context.Context, serverid string, before int64) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tablePresence.DelEqServerIDLeqLastUpdated(ctx, d, serverid, before); err != nil {
		return kerrors.WithMsg(err, "Failed to delete presence")
	}
	return nil
}

func (r *repo) Delete(ctx context.Context, serverid string) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.tablePresence.DelEqServerID(ctx, d, serverid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete presence")
	}
	if err := r.tableChannels.DelEqServerID(ctx, d, serverid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete channels")
	}
	if err := r.table.DelEqServerID(ctx, d, serverid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete server")
	}
	return nil
}

// Setup creates new server, channel, and presence tables
func (r *repo) Setup(ctx context.Context) error {
	d, err := r.db.DB(ctx)
	if err != nil {
		return err
	}
	if err := r.table.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup server model")
		if !errors.Is(err, db.ErrorAuthz) {
			return err
		}
	}
	if err := r.tableChannels.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup server channel model")
		if !errors.Is(err, db.ErrorAuthz) {
			return err
		}
	}
	if err := r.tablePresence.Setup(ctx, d); err != nil {
		err = kerrors.WithMsg(err, "Failed to setup server presence model")
		if !errors.Is(err, db.ErrorAuthz) {
			return err
		}
	}
	return nil
}
