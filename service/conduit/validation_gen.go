// Code generated by go generate forge validation v0.5.2; DO NOT EDIT.

package conduit

func (r reqGetFriends) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validoptUsername(r.Prefix); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

func (r reqSearchFriends) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasUsername(r.Prefix); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	return nil
}

func (r reqRmFriend) valid() error {
	if err := validhasUserid(r.Userid1); err != nil {
		return err
	}
	if err := validhasUserid(r.Userid2); err != nil {
		return err
	}
	return nil
}

func (r reqFriendInvitation) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasUserid(r.InvitedBy); err != nil {
		return err
	}
	return nil
}

func (r reqGetFriendInvitations) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

func (r reqGetLatestChats) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	return nil
}

func (r reqGetChats) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasChatids(r.Chatids); err != nil {
		return err
	}
	return nil
}

func (r reqUpdateDM) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasChatid(r.Chatid); err != nil {
		return err
	}
	if err := validName(r.Name); err != nil {
		return err
	}
	if err := validTheme(r.Theme); err != nil {
		return err
	}
	return nil
}

func (r reqCreateMsg) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasChatid(r.Chatid); err != nil {
		return err
	}
	if err := validMsgkind(r.Kind); err != nil {
		return err
	}
	if err := validMsgvalue(r.Value); err != nil {
		return err
	}
	return nil
}

func (r reqGetMsgs) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasChatid(r.Chatid); err != nil {
		return err
	}
	if err := validoptMsgkind(r.Kind); err != nil {
		return err
	}
	if err := validoptMsgid(r.Before); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	return nil
}

func (r reqDelMsg) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasChatid(r.Chatid); err != nil {
		return err
	}
	if err := validhasMsgid(r.Msgid); err != nil {
		return err
	}
	return nil
}

func (r reqGetPresence) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasUserids(r.Userids); err != nil {
		return err
	}
	return nil
}

func (r reqSearchGDMs) valid() error {
	if err := validhasUserid(r.Userid1); err != nil {
		return err
	}
	if err := validhasUserid(r.Userid2); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

func (r reqCreateGDM) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validName(r.Name); err != nil {
		return err
	}
	if err := validTheme(r.Theme); err != nil {
		return err
	}
	if err := validhasUserids(r.Members); err != nil {
		return err
	}
	return nil
}

func (r reqUpdateGDM) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasChatid(r.Chatid); err != nil {
		return err
	}
	if err := validName(r.Name); err != nil {
		return err
	}
	if err := validTheme(r.Theme); err != nil {
		return err
	}
	return nil
}

func (r reqDelGDM) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasChatid(r.Chatid); err != nil {
		return err
	}
	return nil
}

func (r reqGDMMember) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasChatid(r.Chatid); err != nil {
		return err
	}
	if err := validhasUserids(r.Members); err != nil {
		return err
	}
	return nil
}

func (r reqGetServer) valid() error {
	if err := validhasServerID(r.ServerID); err != nil {
		return err
	}
	return nil
}

func (r reqCreateServer) valid() error {
	if err := validhasServerID(r.ServerID); err != nil {
		return err
	}
	if err := validName(r.Name); err != nil {
		return err
	}
	if err := validDesc(r.Desc); err != nil {
		return err
	}
	if err := validTheme(r.Theme); err != nil {
		return err
	}
	return nil
}

func (r reqGetChannel) valid() error {
	if err := validhasServerID(r.ServerID); err != nil {
		return err
	}
	if err := validhasChannelID(r.ChannelID); err != nil {
		return err
	}
	return nil
}

func (r reqGetChannels) valid() error {
	if err := validhasServerID(r.ServerID); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	if err := validOffset(r.Offset); err != nil {
		return err
	}
	return nil
}

func (r reqSearchChannels) valid() error {
	if err := validhasServerID(r.ServerID); err != nil {
		return err
	}
	if err := validhasChannelID(r.Prefix); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	return nil
}

func (r reqCreateChannel) valid() error {
	if err := validhasServerID(r.ServerID); err != nil {
		return err
	}
	if err := validChannelID(r.ChannelID); err != nil {
		return err
	}
	if err := validName(r.Name); err != nil {
		return err
	}
	if err := validDesc(r.Desc); err != nil {
		return err
	}
	if err := validTheme(r.Theme); err != nil {
		return err
	}
	return nil
}

func (r reqUpdateChannel) valid() error {
	if err := validhasServerID(r.ServerID); err != nil {
		return err
	}
	if err := validhasChannelID(r.ChannelID); err != nil {
		return err
	}
	if err := validName(r.Name); err != nil {
		return err
	}
	if err := validDesc(r.Desc); err != nil {
		return err
	}
	if err := validTheme(r.Theme); err != nil {
		return err
	}
	return nil
}

func (r reqCreateChannelMsg) valid() error {
	if err := validhasUserid(r.Userid); err != nil {
		return err
	}
	if err := validhasServerID(r.ServerID); err != nil {
		return err
	}
	if err := validhasChannelID(r.ChannelID); err != nil {
		return err
	}
	if err := validMsgkind(r.Kind); err != nil {
		return err
	}
	if err := validMsgvalue(r.Value); err != nil {
		return err
	}
	return nil
}

func (r reqGetChannelMsgs) valid() error {
	if err := validhasServerID(r.ServerID); err != nil {
		return err
	}
	if err := validhasChannelID(r.ChannelID); err != nil {
		return err
	}
	if err := validoptMsgkind(r.Kind); err != nil {
		return err
	}
	if err := validoptMsgid(r.Before); err != nil {
		return err
	}
	if err := validAmount(r.Amount); err != nil {
		return err
	}
	return nil
}

func (r reqDelChannelMsg) valid() error {
	if err := validhasServerID(r.ServerID); err != nil {
		return err
	}
	if err := validhasChannelID(r.ChannelID); err != nil {
		return err
	}
	if err := validhasMsgid(r.Msgid); err != nil {
		return err
	}
	return nil
}
