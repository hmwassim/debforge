package definition

import "gopkg.in/yaml.v3"

// MultiString accepts a single string or a list of strings from YAML.
type MultiString []string

func (m *MultiString) UnmarshalYAML(value *yaml.Node) error {
	var single string
	if err := value.Decode(&single); err == nil {
		*m = MultiString{single}
		return nil
	}
	var multi []string
	if err := value.Decode(&multi); err != nil {
		return err
	}
	*m = MultiString(multi)
	return nil
}
