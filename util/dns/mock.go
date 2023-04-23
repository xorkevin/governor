package dns

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/kerrors"
)

type (
	MockZone struct {
		A     []string `json:"A"`
		AAAA  []string `json:"AAAA"`
		MX    []net.MX `json:"MX"`
		TXT   []string `json:"TXT"`
		CNAME string   `json:"CNAME"`
	}

	MockResolver struct {
		Zones map[string]MockZone
	}

	zoneData struct {
		Data map[string]MockZone `json:"data"`
	}
)

var (
	// ErrNotFound is returned when a record is not found
	ErrNotFound errNotFound
	// ErrInvalid is returned when a record is invalid
	ErrInvalid errInvalid
)

type (
	errNotFound struct{}
	errInvalid  struct{}
)

func (e errNotFound) Error() string {
	return "DNS record not found"
}

func (e errInvalid) Error() string {
	return "DNS record is invalid"
}

func IsFQDN(s string) bool {
	return s != strings.TrimSuffix(s, ".")
}

func FQDN(s string) string {
	if IsFQDN(s) {
		return strings.ToLower(s)
	}
	return strings.ToLower(s) + "."
}

func NewMockResolver(zones map[string]MockZone) Resolver {
	return &MockResolver{
		Zones: zones,
	}
}

func NewMockResolverFromFile(s string) (Resolver, error) {
	b, err := os.ReadFile(s)
	if err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Failed to read mockdns file %s", s))
	}
	data := zoneData{}
	if err := kjson.Unmarshal(b, &data); err != nil {
		return nil, kerrors.WithMsg(err, fmt.Sprintf("Invalid mockdns file %s", s))
	}
	return &MockResolver{
		Zones: data.Data,
	}, nil
}

func (r *MockResolver) LookupAddr(ctx context.Context, addr string) ([]string, error) {
	return nil, ErrNotFound
}

func (r *MockResolver) LookupCNAME(ctx context.Context, host string) (cname string, err error) {
	z, ok := r.Zones[FQDN(host)]
	if !ok {
		return "", ErrNotFound
	}
	return z.CNAME, nil
}

func (r *MockResolver) targetZone(name string) (MockZone, error) {
	name = FQDN(name)
	z, ok := r.Zones[name]
	if !ok {
		return MockZone{}, ErrNotFound
	}
	for z.CNAME != "" {
		name = z.CNAME
		z, ok = r.Zones[name]
		if !ok {
			return MockZone{}, ErrNotFound
		}
	}
	return z, nil
}

func (r *MockResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	z, err := r.targetZone(host)
	if err != nil {
		return nil, err
	}
	res := make([]string, 0, len(z.A)+len(z.AAAA))
	res = append(res, z.A...)
	res = append(res, z.AAAA...)
	if len(res) == 0 {
		return nil, ErrNotFound
	}
	return res, err
}

func (r *MockResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	addrs, err := r.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}
	res := make([]net.IPAddr, 0, len(addrs))
	for _, i := range addrs {
		ip := net.ParseIP(i)
		if ip == nil {
			return nil, kerrors.WithKind(nil, ErrInvalid, fmt.Sprintf("Invalid IP %s", i))
		}
		res = append(res, net.IPAddr{IP: ip})
	}
	return res, nil
}

func (r *MockResolver) LookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	z, err := r.targetZone(name)
	if err != nil {
		return nil, err
	}
	res := make([]*net.MX, 0, len(z.MX))
	for _, i := range z.MX {
		k := i
		res = append(res, &k)
	}
	return res, nil
}

func (r *MockResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	z, err := r.targetZone(name)
	if err != nil {
		return nil, err
	}
	res := make([]string, len(z.TXT))
	copy(res, z.TXT)
	return res, nil
}
