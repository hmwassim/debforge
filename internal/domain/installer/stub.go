package installer

import "fmt"

func StubError(typ string) error {
	return fmt.Errorf("%s installer: not implemented", typ)
}
