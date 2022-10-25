package conduit

import (
	"net/http"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

type (
	//forge:valid
	reqGetFriends struct {
		Userid string `valid:"userid,has" json:"-"`
		Prefix string `valid:"username,opt" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (s *router) getFriends(c *governor.Context) {
	req := reqGetFriends{
		Userid: gate.GetCtxUserid(c),
		Prefix: c.Query("prefix"),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getFriends(c.Ctx(), req.Userid, req.Prefix, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqSearchFriends struct {
		Userid string `valid:"userid,has" json:"-"`
		Prefix string `valid:"username,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
	}
)

func (s *router) searchFriends(c *governor.Context) {
	req := reqSearchFriends{
		Userid: gate.GetCtxUserid(c),
		Prefix: c.Query("prefix"),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.searchFriends(c.Ctx(), req.Userid, req.Prefix, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqRmFriend struct {
		Userid1 string `valid:"userid,has" json:"-"`
		Userid2 string `valid:"userid,has" json:"-"`
	}
)

func (s *router) removeFriend(c *governor.Context) {
	req := reqRmFriend{
		Userid1: gate.GetCtxUserid(c),
		Userid2: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.removeFriend(c.Ctx(), req.Userid1, req.Userid2); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqFriendInvitation struct {
		Userid    string `valid:"userid,has" json:"-"`
		InvitedBy string `valid:"userid,has" json:"-"`
	}
)

func (s *router) sendFriendInvitation(c *governor.Context) {
	req := reqFriendInvitation{
		Userid:    c.Param("id"),
		InvitedBy: gate.GetCtxUserid(c),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.inviteFriend(c.Ctx(), req.Userid, req.InvitedBy); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) acceptFriendInvitation(c *governor.Context) {
	req := reqFriendInvitation{
		Userid:    gate.GetCtxUserid(c),
		InvitedBy: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.acceptFriendInvitation(c.Ctx(), req.Userid, req.InvitedBy); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) deleteUserFriendInvitation(c *governor.Context) {
	req := reqFriendInvitation{
		Userid:    gate.GetCtxUserid(c),
		InvitedBy: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.deleteFriendInvitation(c.Ctx(), req.Userid, req.InvitedBy); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) deleteInvitedFriendInvitation(c *governor.Context) {
	req := reqFriendInvitation{
		Userid:    c.Param("id"),
		InvitedBy: gate.GetCtxUserid(c),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.deleteFriendInvitation(c.Ctx(), req.Userid, req.InvitedBy); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqGetFriendInvitations struct {
		Userid string `valid:"userid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (s *router) getInvitations(c *governor.Context) {
	req := reqGetFriendInvitations{
		Userid: gate.GetCtxUserid(c),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getUserFriendInvitations(c.Ctx(), req.Userid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getInvited(c *governor.Context) {
	req := reqGetFriendInvitations{
		Userid: gate.GetCtxUserid(c),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getInvitedFriendInvitations(c.Ctx(), req.Userid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqGetLatestChats struct {
		Userid string `valid:"userid,has" json:"-"`
		Before int64  `json:"-"`
		Amount int    `valid:"amount" json:"-"`
	}
)

func (s *router) getLatestDMs(c *governor.Context) {
	req := reqGetLatestChats{
		Userid: gate.GetCtxUserid(c),
		Before: c.QueryInt64("before", 0),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getLatestDMs(c.Ctx(), req.Userid, req.Before, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqGetChats struct {
		Userid  string   `valid:"userid,has" json:"-"`
		Chatids []string `valid:"chatids,has" json:"-"`
	}
)

func (s *router) getDMs(c *governor.Context) {
	req := reqGetChats{
		Userid:  gate.GetCtxUserid(c),
		Chatids: strings.Split(c.Query("ids"), ","),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getDMs(c.Ctx(), req.Userid, req.Chatids)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) searchDMs(c *governor.Context) {
	req := reqSearchFriends{
		Userid: gate.GetCtxUserid(c),
		Prefix: c.Query("prefix"),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.searchDMs(c.Ctx(), req.Userid, req.Prefix, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqUpdateDM struct {
		Userid string `valid:"userid,has" json:"-"`
		Chatid string `valid:"chatid,has" json:"-"`
		Name   string `valid:"name" json:"name"`
		Theme  string `valid:"theme" json:"theme"`
	}
)

func (s *router) updateDM(c *governor.Context) {
	var req reqUpdateDM
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	req.Chatid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.updateDM(c.Ctx(), req.Userid, req.Chatid, req.Name, req.Theme); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqCreateMsg struct {
		Userid string `valid:"userid,has" json:"-"`
		Chatid string `valid:"chatid,has" json:"-"`
		Kind   string `valid:"msgkind" json:"kind"`
		Value  string `valid:"msgvalue" json:"value"`
	}
)

func (s *router) createDMMsg(c *governor.Context) {
	var req reqCreateMsg
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	req.Chatid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.createDMMsg(c.Ctx(), req.Userid, req.Chatid, req.Kind, req.Value)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	//forge:valid
	reqGetMsgs struct {
		Userid string `valid:"userid,has" json:"-"`
		Chatid string `valid:"chatid,has" json:"-"`
		Kind   string `valid:"msgkind,opt" json:"-"`
		Before string `valid:"msgid,opt" json:"-"`
		Amount int    `valid:"amount" json:"-"`
	}
)

func (s *router) getDMMsgs(c *governor.Context) {
	req := reqGetMsgs{
		Userid: gate.GetCtxUserid(c),
		Chatid: c.Param("id"),
		Kind:   c.Query("kind"),
		Before: c.Query("before"),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getDMMsgs(c.Ctx(), req.Userid, req.Chatid, req.Kind, req.Before, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqDelMsg struct {
		Userid string `valid:"userid,has" json:"-"`
		Chatid string `valid:"chatid,has" json:"-"`
		Msgid  string `valid:"msgid,has" json:"-"`
	}
)

func (s *router) deleteDMMsg(c *governor.Context) {
	req := reqDelMsg{
		Userid: gate.GetCtxUserid(c),
		Chatid: c.Param("id"),
		Msgid:  c.Param("msgid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.delDMMsg(c.Ctx(), req.Userid, req.Chatid, req.Msgid); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqGetPresence struct {
		Userid  string   `valid:"userid,has" json:"-"`
		Userids []string `valid:"userids,has" json:"userids"`
	}
)

func (s *router) getLatestGDMs(c *governor.Context) {
	req := reqGetLatestChats{
		Userid: gate.GetCtxUserid(c),
		Before: c.QueryInt64("before", 0),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getLatestGDMs(c.Ctx(), req.Userid, req.Before, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getGDMs(c *governor.Context) {
	req := reqGetChats{
		Userid:  gate.GetCtxUserid(c),
		Chatids: strings.Split(c.Query("ids"), ","),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getGDMs(c.Ctx(), req.Userid, req.Chatids)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqSearchGDMs struct {
		Userid1 string `valid:"userid,has" json:"-"`
		Userid2 string `valid:"userid,has" json:"-"`
		Amount  int    `valid:"amount" json:"-"`
		Offset  int    `valid:"offset" json:"-"`
	}
)

func (s *router) searchGDMs(c *governor.Context) {
	req := reqSearchGDMs{
		Userid1: gate.GetCtxUserid(c),
		Userid2: c.Query("id"),
		Amount:  c.QueryInt("amount", -1),
		Offset:  c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.searchGDMs(c.Ctx(), req.Userid1, req.Userid2, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqCreateGDM struct {
		Userid  string   `valid:"userid,has" json:"-"`
		Name    string   `valid:"name" json:"name"`
		Theme   string   `valid:"theme" json:"theme"`
		Members []string `valid:"userids,has" json:"members"`
	}
)

func (s *router) createGDM(c *governor.Context) {
	var req reqCreateGDM
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	members := make([]string, len(req.Members)+1)
	members[0] = req.Userid
	copy(members[1:], req.Members)
	res, err := s.s.createGDM(c.Ctx(), req.Name, req.Theme, members)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	//forge:valid
	reqUpdateGDM struct {
		Userid string `valid:"userid,has" json:"-"`
		Chatid string `valid:"chatid,has" json:"-"`
		Name   string `valid:"name" json:"name"`
		Theme  string `valid:"theme" json:"theme"`
	}
)

func (s *router) updateGDM(c *governor.Context) {
	var req reqUpdateGDM
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	req.Chatid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.updateGDM(c.Ctx(), req.Userid, req.Chatid, req.Name, req.Theme); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqDelGDM struct {
		Userid string `valid:"userid,has" json:"-"`
		Chatid string `valid:"chatid,has" json:"-"`
	}
)

func (s *router) deleteGDM(c *governor.Context) {
	req := reqDelGDM{
		Userid: gate.GetCtxUserid(c),
		Chatid: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.deleteGDM(c.Ctx(), req.Userid, req.Chatid); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) createGDMMsg(c *governor.Context) {
	var req reqCreateMsg
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	req.Chatid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.createGDMMsg(c.Ctx(), req.Userid, req.Chatid, req.Kind, req.Value)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

func (s *router) getGDMMsgs(c *governor.Context) {
	req := reqGetMsgs{
		Userid: gate.GetCtxUserid(c),
		Chatid: c.Param("id"),
		Kind:   c.Query("kind"),
		Before: c.Query("before"),
		Amount: c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getGDMMsgs(c.Ctx(), req.Userid, req.Chatid, req.Kind, req.Before, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) deleteGDMMsg(c *governor.Context) {
	req := reqDelMsg{
		Userid: gate.GetCtxUserid(c),
		Chatid: c.Param("id"),
		Msgid:  c.Param("msgid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.delGDMMsg(c.Ctx(), req.Userid, req.Chatid, req.Msgid); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqGDMMember struct {
		Userid  string   `valid:"userid,has" json:"-"`
		Chatid  string   `valid:"chatid,has" json:"-"`
		Members []string `valid:"userids,has" json:"members"`
	}
)

func (s *router) addGDMMember(c *governor.Context) {
	var req reqGDMMember
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	req.Chatid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.addGDMMembers(c.Ctx(), req.Userid, req.Chatid, req.Members); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) rmGDMMembers(c *governor.Context) {
	var req reqGDMMember
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	req.Chatid = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.rmGDMMembers(c.Ctx(), req.Userid, req.Chatid, req.Members); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqGetServer struct {
		ServerID string `valid:"serverID,has" json:"-"`
	}
)

func (s *router) getServer(c *governor.Context) {
	req := reqGetServer{
		ServerID: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getServer(c.Ctx(), req.ServerID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqCreateServer struct {
		ServerID string `valid:"serverID,has" json:"-"`
		Name     string `valid:"name" json:"name"`
		Desc     string `valid:"desc" json:"desc"`
		Theme    string `valid:"theme" json:"theme"`
	}
)

func (s *router) createServer(c *governor.Context) {
	var req reqCreateServer
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.ServerID = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.createServer(c.Ctx(), req.ServerID, req.Name, req.Desc, req.Theme)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

func (s *router) updateServer(c *governor.Context) {
	var req reqCreateServer
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.ServerID = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.updateServer(c.Ctx(), req.ServerID, req.Name, req.Desc, req.Theme); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqGetChannel struct {
		ServerID  string `valid:"serverID,has" json:"-"`
		ChannelID string `valid:"channelID,has" json:"-"`
	}
)

func (s *router) getChannel(c *governor.Context) {
	req := reqGetChannel{
		ServerID:  c.Param("id"),
		ChannelID: c.Param("cid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getChannel(c.Ctx(), req.ServerID, req.ChannelID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqGetChannels struct {
		ServerID string `valid:"serverID,has" json:"-"`
		Amount   int    `valid:"amount" json:"-"`
		Offset   int    `valid:"offset" json:"-"`
	}
)

func (s *router) getChannels(c *governor.Context) {
	req := reqGetChannels{
		ServerID: c.Param("id"),
		Amount:   c.QueryInt("amount", -1),
		Offset:   c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getChannels(c.Ctx(), req.ServerID, "", req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqSearchChannels struct {
		ServerID string `valid:"serverID,has" json:"-"`
		Prefix   string `valid:"channelID,has" json:"-"`
		Amount   int    `valid:"amount" json:"-"`
	}
)

func (s *router) searchChannels(c *governor.Context) {
	req := reqSearchChannels{
		ServerID: c.Param("id"),
		Prefix:   c.Query("prefix"),
		Amount:   c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getChannels(c.Ctx(), req.ServerID, req.Prefix, req.Amount, 0)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqCreateChannel struct {
		ServerID  string `valid:"serverID,has" json:"-"`
		ChannelID string `valid:"channelID" json:"channelid"`
		Name      string `valid:"name" json:"name"`
		Desc      string `valid:"desc" json:"desc"`
		Theme     string `valid:"theme" json:"theme"`
	}
)

func (s *router) createChannel(c *governor.Context) {
	var req reqCreateChannel
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.ServerID = c.Param("id")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.createChannel(c.Ctx(), req.ServerID, req.ChannelID, req.Name, req.Desc, req.Theme)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	//forge:valid
	reqUpdateChannel struct {
		ServerID  string `valid:"serverID,has" json:"-"`
		ChannelID string `valid:"channelID,has" json:"-"`
		Name      string `valid:"name" json:"name"`
		Desc      string `valid:"desc" json:"desc"`
		Theme     string `valid:"theme" json:"theme"`
	}
)

func (s *router) updateChannel(c *governor.Context) {
	var req reqUpdateChannel
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.ServerID = c.Param("id")
	req.ChannelID = c.Param("cid")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.updateChannel(c.Ctx(), req.ServerID, req.ChannelID, req.Name, req.Desc, req.Theme); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) deleteChannel(c *governor.Context) {
	req := reqGetChannel{
		ServerID:  c.Param("id"),
		ChannelID: c.Param("cid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.deleteChannel(c.Ctx(), req.ServerID, req.ChannelID); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

type (
	//forge:valid
	reqCreateChannelMsg struct {
		Userid    string `valid:"userid,has" json:"-"`
		ServerID  string `valid:"serverID,has" json:"-"`
		ChannelID string `valid:"channelID,has" json:"-"`
		Kind      string `valid:"msgkind" json:"kind"`
		Value     string `valid:"msgvalue" json:"value"`
	}
)

func (s *router) createChannelMsg(c *governor.Context) {
	var req reqCreateChannelMsg
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	req.ServerID = c.Param("id")
	req.ChannelID = c.Param("cid")
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.createChannelMsg(c.Ctx(), req.ServerID, req.ChannelID, req.Userid, req.Kind, req.Value)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusCreated, res)
}

type (
	//forge:valid
	reqGetChannelMsgs struct {
		ServerID  string `valid:"serverID,has" json:"-"`
		ChannelID string `valid:"channelID,has" json:"-"`
		Kind      string `valid:"msgkind,opt" json:"-"`
		Before    string `valid:"msgid,opt" json:"-"`
		Amount    int    `valid:"amount" json:"-"`
	}
)

func (s *router) getChannelMsgs(c *governor.Context) {
	req := reqGetChannelMsgs{
		ServerID:  c.Param("id"),
		ChannelID: c.Param("cid"),
		Kind:      c.Query("kind"),
		Before:    c.Query("before"),
		Amount:    c.QueryInt("amount", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	res, err := s.s.getChannelMsgs(c.Ctx(), req.ServerID, req.ChannelID, req.Kind, req.Before, req.Amount)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqDelChannelMsg struct {
		ServerID  string `valid:"serverID,has" json:"-"`
		ChannelID string `valid:"channelID,has" json:"-"`
		Msgid     string `valid:"msgid,has" json:"-"`
	}
)

func (s *router) deleteChannelMsg(c *governor.Context) {
	req := reqDelChannelMsg{
		ServerID:  c.Param("id"),
		ChannelID: c.Param("cid"),
		Msgid:     c.Param("msgid"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if err := s.s.deleteChannelMsg(c.Ctx(), req.ServerID, req.ChannelID, req.Msgid); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) serverMember(c *governor.Context, userid string) (string, bool, bool) {
	serverid := c.Param("id")
	if err := validhasServerID(serverid); err != nil {
		return "", false, false
	}
	return rank.ToOrgName(serverid), false, true
}

func (s *router) mountRoutes(r governor.Router) {
	m := governor.NewMethodRouter(r)

	scopeFriendRead := s.s.scopens + ".friend:read"
	scopeFriendWrite := s.s.scopens + ".friend:write"
	m.GetCtx("/friend", s.getFriends, gate.User(s.s.gate, scopeFriendRead), s.rt)
	m.DeleteCtx("/friend/id/{id}", s.removeFriend, gate.User(s.s.gate, scopeFriendWrite), s.rt)
	m.GetCtx("/friend/search", s.searchFriends, gate.User(s.s.gate, scopeFriendRead), s.rt)
	m.GetCtx("/friend/invitation", s.getInvitations, gate.User(s.s.gate, scopeFriendRead), s.rt)
	m.GetCtx("/friend/invitation/invited", s.getInvited, gate.User(s.s.gate, scopeFriendRead), s.rt)
	m.PostCtx("/friend/invitation/id/{id}", s.sendFriendInvitation, gate.User(s.s.gate, scopeFriendWrite), s.rt)
	m.PostCtx("/friend/invitation/id/{id}/accept", s.acceptFriendInvitation, gate.User(s.s.gate, scopeFriendWrite), s.rt)
	m.DeleteCtx("/friend/invitation/id/{id}", s.deleteUserFriendInvitation, gate.User(s.s.gate, scopeFriendWrite), s.rt)
	m.DeleteCtx("/friend/invitation/invited/{id}", s.deleteInvitedFriendInvitation, gate.User(s.s.gate, scopeFriendWrite), s.rt)

	scopeChatRead := s.s.scopens + ".chat:read"
	scopeChatWrite := s.s.scopens + ".chat:write"
	scopeChatAdminWrite := s.s.scopens + ".chat.admin:write"
	m.GetCtx("/dm", s.getLatestDMs, gate.User(s.s.gate, scopeChatRead), s.rt)
	m.GetCtx("/dm/ids", s.getDMs, gate.User(s.s.gate, scopeChatRead), s.rt)
	m.GetCtx("/dm/search", s.searchDMs, gate.User(s.s.gate, scopeChatRead), s.rt)
	m.PutCtx("/dm/id/{id}", s.updateDM, gate.User(s.s.gate, scopeChatAdminWrite), s.rt)
	m.PostCtx("/dm/id/{id}/msg", s.createDMMsg, gate.User(s.s.gate, scopeChatWrite), s.rt)
	m.GetCtx("/dm/id/{id}/msg", s.getDMMsgs, gate.User(s.s.gate, scopeChatRead), s.rt)
	m.DeleteCtx("/dm/id/{id}/msg/id/{msgid}", s.deleteDMMsg, gate.User(s.s.gate, scopeChatWrite), s.rt)

	m.GetCtx("/gdm", s.getLatestGDMs, gate.User(s.s.gate, scopeChatRead), s.rt)
	m.GetCtx("/gdm/ids", s.getGDMs, gate.User(s.s.gate, scopeChatRead), s.rt)
	m.GetCtx("/gdm/search", s.searchGDMs, gate.User(s.s.gate, scopeChatRead), s.rt)
	m.PostCtx("/gdm", s.createGDM, gate.User(s.s.gate, scopeChatAdminWrite), s.rt)
	m.PutCtx("/gdm/id/{id}", s.updateGDM, gate.User(s.s.gate, scopeChatAdminWrite), s.rt)
	m.DeleteCtx("/gdm/id/{id}", s.deleteGDM, gate.User(s.s.gate, scopeChatAdminWrite), s.rt)
	m.PatchCtx("/gdm/id/{id}/member/add", s.addGDMMember, gate.User(s.s.gate, scopeChatAdminWrite), s.rt)
	m.PatchCtx("/gdm/id/{id}/member/rm", s.rmGDMMembers, gate.User(s.s.gate, scopeChatAdminWrite), s.rt)
	m.PostCtx("/gdm/id/{id}/msg", s.createGDMMsg, gate.User(s.s.gate, scopeChatWrite), s.rt)
	m.GetCtx("/gdm/id/{id}/msg", s.getGDMMsgs, gate.User(s.s.gate, scopeChatRead), s.rt)
	m.DeleteCtx("/gdm/id/{id}/msg/id/{msgid}", s.deleteGDMMsg, gate.User(s.s.gate, scopeChatWrite), s.rt)

	scopeServerRead := s.s.scopens + ".server:read"
	scopeServerWrite := s.s.scopens + ".server:write"
	scopeServerChatWrite := s.s.scopens + ".server.chat:write"
	m.GetCtx("/server/id/{id}", s.getServer, gate.MemberF(s.s.gate, s.serverMember, scopeServerRead), s.rt)
	m.PostCtx("/server/id/{id}", s.createServer, gate.MemberF(s.s.gate, s.serverMember, scopeServerWrite), s.rt)
	m.PutCtx("/server/id/{id}", s.updateServer, gate.MemberF(s.s.gate, s.serverMember, scopeServerWrite), s.rt)
	m.GetCtx("/server/id/{id}/channel/id/{cid}", s.getChannel, gate.MemberF(s.s.gate, s.serverMember, scopeServerRead), s.rt)
	m.GetCtx("/server/id/{id}/channel", s.getChannels, gate.MemberF(s.s.gate, s.serverMember, scopeServerRead), s.rt)
	m.GetCtx("/server/id/{id}/channel/search", s.searchChannels, gate.MemberF(s.s.gate, s.serverMember, scopeServerRead), s.rt)
	m.PostCtx("/server/id/{id}/channel", s.createChannel, gate.MemberF(s.s.gate, s.serverMember, scopeServerWrite), s.rt)
	m.PutCtx("/server/id/{id}/channel/id/{cid}", s.updateChannel, gate.MemberF(s.s.gate, s.serverMember, scopeServerWrite), s.rt)
	m.DeleteCtx("/server/id/{id}/channel/id/{cid}", s.deleteChannel, gate.MemberF(s.s.gate, s.serverMember, scopeServerWrite), s.rt)
	m.PostCtx("/server/id/{id}/channel/id/{cid}/msg", s.createChannelMsg, gate.MemberF(s.s.gate, s.serverMember, scopeServerChatWrite), s.rt)
	m.GetCtx("/server/id/{id}/channel/id/{cid}/msg", s.getChannelMsgs, gate.MemberF(s.s.gate, s.serverMember, scopeServerRead), s.rt)
	m.DeleteCtx("/server/id/{id}/channel/id/{cid}/msg/id/{msgid}", s.deleteChannelMsg, gate.MemberF(s.s.gate, s.serverMember, scopeServerChatWrite), s.rt)
}
