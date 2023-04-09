// Code generated by go generate forge validation v0.4.2; DO NOT EDIT.

package courier

func (r reqLinkGet) valid() error {
	if err := validhasLinkID(r.LinkID); err != nil {
		return err
	}
	return nil
}

func (r reqGetGroup) valid() error {
	if err := validhasCreatorID(r.CreatorID); err != nil {
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

func (r reqLinkPost) valid() error {
	if err := validhasCreatorID(r.CreatorID); err != nil {
		return err
	}
	if err := validLinkID(r.LinkID); err != nil {
		return err
	}
	if err := validURL(r.URL); err != nil {
		return err
	}
	if err := validhasBrandID(r.BrandID); err != nil {
		return err
	}
	return nil
}

func (r reqLinkDelete) valid() error {
	if err := validhasCreatorID(r.CreatorID); err != nil {
		return err
	}
	if err := validhasLinkID(r.LinkID); err != nil {
		return err
	}
	return nil
}

func (r reqBrandGet) valid() error {
	if err := validhasCreatorID(r.CreatorID); err != nil {
		return err
	}
	if err := validhasBrandID(r.BrandID); err != nil {
		return err
	}
	return nil
}

func (r reqBrandPost) valid() error {
	if err := validhasCreatorID(r.CreatorID); err != nil {
		return err
	}
	if err := validBrandID(r.BrandID); err != nil {
		return err
	}
	return nil
}
