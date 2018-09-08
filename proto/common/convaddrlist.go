package common

import "net/mail"

func ConvertAddrList(in []*mail.Address, err error) ([]Address, error) {
	if err != nil {
		return nil, err
	}
	res := make([]Address, len(in))
	for i, a := range in {
		res[i] = Address(*a)
	}
	return res, nil
}
