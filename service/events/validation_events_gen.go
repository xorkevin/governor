// Code generated by go generate forge validation v0.3; DO NOT EDIT.

package events

func (r reqPublishEvent) valid() error {
	if err := validSubject(r.Subject); err != nil {
		return err
	}
	return nil
}
