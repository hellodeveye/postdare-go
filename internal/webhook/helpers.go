package webhook

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func stringFromMap(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func mapFromMap(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return nil
	}
	child, _ := m[key].(map[string]interface{})
	return child
}
