package decoder

type getter func(input *DecodeInput, key string, defaultValue ...string) (ret string, exist bool)

func path(input *DecodeInput, key string, defaultValue ...string) (ret string, exist bool) {
	if input.Params != nil {
		ret, exist = input.Params.Get(key)
	}

	if len(ret) == 0 && len(defaultValue) != 0 {
		ret = defaultValue[0]
	}

	return ret, exist
}

func postForm(input *DecodeInput, key string, defaultValue ...string) (ret string, exist bool) {
	if ret, exist = peekArgStrExists(input.Request.PostForm, key); exist {
		return
	}

	for k, v := range input.Request.MultipartForm.Value {
		if k == key && len(v) > 0 {
			ret = v[0]
		}
	}

	if len(ret) != 0 {
		return ret, true
	}

	if ret, exist = peekArgStrExists(input.Request.Form, key); exist {
		return
	}

	if ret, exist = peekArgStrExists(input.Request.URL.Query(), key); exist {
		return
	}

	if len(ret) == 0 && len(defaultValue) != 0 {
		ret = defaultValue[0]
	}

	return ret, false
}

func query(input *DecodeInput, key string, defaultValue ...string) (ret string, exist bool) {
	if ret, exist = peekArgStrExists(input.Request.URL.Query(), key); exist {
		return
	}
	if len(ret) == 0 && len(defaultValue) != 0 {
		ret = defaultValue[0]
	}

	return
}

func cookie(input *DecodeInput, key string, defaultValue ...string) (ret string, exist bool) {
	if val, _ := input.Request.Cookie(key); val != nil {
		ret = val.Value
		return ret, true
	}

	if len(ret) == 0 && len(defaultValue) != 0 {
		ret = defaultValue[0]
	}

	return ret, false
}

func header(input *DecodeInput, key string, defaultValue ...string) (ret string, exist bool) {
	if val := input.Request.Header.Get(key); val != "" {
		ret = val
		return ret, true
	}

	if len(ret) == 0 && len(defaultValue) != 0 {
		ret = defaultValue[0]
	}

	return ret, false
}

func rawBody(input *DecodeInput, key string, defaultValue ...string) (ret string, exist bool) {
	exist = false
	if input.ContentLength() > 0 {
		ret = string(input.Body())
		exist = true
	}
	return
}
