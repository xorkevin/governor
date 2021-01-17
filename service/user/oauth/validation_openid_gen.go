// Code generated by go generate forge validation v0.3; DO NOT EDIT.

package oauth

func (r reqOidAuthorize) valid() error {
	if err := validhasClientID(r.ClientID); err != nil {
		return err
	}
	if err := validOidScope(r.Scope); err != nil {
		return err
	}
	if err := validOidNonce(r.Nonce); err != nil {
		return err
	}
	if err := validOidCodeChallenge(r.CodeChallenge); err != nil {
		return err
	}
	if err := validOidCodeChallengeMethod(r.CodeChallengeMethod); err != nil {
		return err
	}
	return nil
}