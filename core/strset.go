package core

type StrSet map[string]bool

func (ss StrSet) List() []string {
	var res []string
	for k := range ss {
		res = append(res, k)
	}
	return res
}

func (ss StrSet) Present(key string) bool {
	_, prs := ss[key]
	return prs
}

func (ss StrSet) Add(key string) {
	ss[key] = true
}

func (ss StrSet) Remove(key string) {
	delete(ss, key)
}
