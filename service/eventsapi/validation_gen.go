// Code generated by go generate forge validation v0.5.2; DO NOT EDIT.

package eventsapi

func (r reqPublishEvent) valid() error {
	if err := validSubject(r.Subject); err != nil {
		return err
	}
	return nil
}
