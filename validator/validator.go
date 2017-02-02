package validator

import "distudio.com/mage"

type Validator interface {
	Validate(value string) bool
	ErrorMessage() string
}

type Field struct {
	Name string
	validators []Validator
	Required bool
	Valid bool
	ErrorMissing string
	value string
	hasValue bool
}

func NewField(name string, in mage.RequestInputs) *Field {
	vs := make([]Validator, 0, 0);
	f := &Field{Name:name, validators:vs};
	f.Valid = false;
	_, hasValue := in[f.Name];
	f.hasValue = hasValue;
	if hasValue {
		f.value = in[f.Name].Value();
	}
	return f;
}

func (field *Field) AddValidator(v Validator) {
	field.validators = append(field.validators, v);
}

func (field *Field) Validate() string {

	field.Valid = false;
	if field.Required {
		if !field.hasValue || field.value == "" {
			return field.ErrorMissing;
		}

	}


	for _, v := range field.validators {
		if !v.Validate(field.value) {
			return v.ErrorMessage();
		}
	}

	field.Valid = true;
	return "";
}

//safe to call after validate
func (field *Field) Value() string {
	if !field.Valid {
		return "";
	}
	return field.value;
}