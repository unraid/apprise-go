package notify

type SchemaValue struct {
	Value any
}

func schemaValueString(value string) SchemaValue {
	return SchemaValue{Value: value}
}

func schemaValueList(values []string) SchemaValue {
	return SchemaValue{Value: values}
}

func schemaValueBool(value bool) SchemaValue {
	return SchemaValue{Value: value}
}

func schemaValueInt(value int) SchemaValue {
	return SchemaValue{Value: value}
}

func schemaValueFloat(value float64) SchemaValue {
	return SchemaValue{Value: value}
}

func schemaValueAny(value any) SchemaValue {
	return SchemaValue{Value: value}
}
