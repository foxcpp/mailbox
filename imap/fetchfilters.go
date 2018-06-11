package imap

func TextOnly(t, sub string) bool {
	return t == "text"
}

func PlainTextOnly(t, sub string) bool {
	return t == "text" && sub == "plain"
}

func HTMLOnly(t, sub string) bool {
	return t == "text" && sub == "html"
}

func OrCombine(funcs ...func(string, string) bool) func(string, string) bool {
	return func(t, s string) bool {
		for _, f := range funcs {
			if f(t, s) {
				return true
			}
		}
		return false
	}
}
