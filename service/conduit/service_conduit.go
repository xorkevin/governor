package conduit

import (
	"xorkevin.dev/governor"
)

type (
	resChat struct {
		ChatID       string `json:"chatid"`
		Kind         string `json:"kind"`
		Name         string `json:"name"`
		Theme        string `json:"theme"`
		LastUpdated  int64  `json:"last_updated"`
		CreationTime int64  `json:"creation_time"`
	}
)

func (s *service) CreateChat(kind string, name string, theme string) (*resChat, error) {
	m, err := s.repo.NewChat(kind, name, theme)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new chat id")
	}
	if err := s.repo.InsertChat(m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new chat")
	}
	return &resChat{
		ChatID:       m.Chatid,
		Kind:         m.Chatid,
		Name:         m.Name,
		Theme:        m.Theme,
		LastUpdated:  m.LastUpdated,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) CreateChatWithUsers(kind string, name string, theme string, userids []string) (*resChat, error) {
	m, err := s.repo.NewChat(kind, name, theme)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new chat id")
	}
	members := s.repo.AddMembers(m, userids)
	if err := s.repo.InsertChat(m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new chat")
	}
	if err := s.repo.InsertMembers(members); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to add chat members")
	}
	return &resChat{
		ChatID:       m.Chatid,
		Kind:         m.Chatid,
		Name:         m.Name,
		Theme:        m.Theme,
		LastUpdated:  m.LastUpdated,
		CreationTime: m.CreationTime,
	}, nil
}
