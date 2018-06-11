package common

import "net/mail"

func ConvertAddrList(in []*mail.Address, err error) ([]mail.Address, error) {
	if err != nil {
		return nil, err
	}
	res := make([]mail.Address, len(in))
	for i, a := range in {
		res[i] = *a
	}
	return res, nil
}
