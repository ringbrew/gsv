package decoder

type sliceGetter func(input *DecodeInput, key string, defaultValue ...string) (ret []string)

func pathSlice(input *DecodeInput, key string, defaultValue ...string) (ret []string) {
	var value string
	if input.Params != nil {
		value, _ = input.Params.Get(key)
	}

	if len(value) == 0 && len(defaultValue) != 0 {
		value = defaultValue[0]
	}
	if len(value) != 0 {
		ret = append(ret, value)
	}

	return
}

func postFormSlice(input *DecodeInput, key string, defaultValue ...string) (ret []string) {
	for k, v := range input.Request.PostForm {
		if k == key {
			ret = append(ret, v...)
		}
	}

	if len(ret) > 0 {
		return
	}

	if input.Request.MultipartForm != nil {
		for k, v := range input.Request.MultipartForm.Value {
			if k == key {
				ret = append(ret, v...)
			}
		}
	}

	if len(ret) > 0 {
		return
	}

	if len(ret) == 0 && len(defaultValue) != 0 {
		ret = append(ret, defaultValue...)
	}

	return
}

func querySlice(input *DecodeInput, key string, defaultValue ...string) (ret []string) {
	for k, v := range input.Request.URL.Query() {
		if k == key {
			ret = append(ret, v...)
		}
	}

	if len(ret) == 0 && len(defaultValue) != 0 {
		ret = append(ret, defaultValue...)
	}

	return
}

func cookieSlice(input *DecodeInput, key string, defaultValue ...string) (ret []string) {
	for _, v := range input.Request.Cookies() {
		if v.Name == key {
			ret = append(ret, v.Value)
		}
	}

	if len(ret) == 0 && len(defaultValue) != 0 {
		ret = append(ret, defaultValue...)
	}

	return
}

func headerSlice(input *DecodeInput, key string, defaultValue ...string) (ret []string) {
	for headerKey, v := range input.Request.Header {
		if headerKey == key {
			ret = append(ret, v...)
		}
	}

	if len(ret) == 0 && len(defaultValue) != 0 {
		ret = append(ret, defaultValue...)
	}

	return
}

func rawBodySlice(input *DecodeInput, key string, defaultValue ...string) (ret []string) {
	if input.ContentLength() > 0 {
		ret = append(ret, string(input.Body()))
	}
	return
}
