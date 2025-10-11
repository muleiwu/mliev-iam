package assembly

import (
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/env/impl"
)

type Env struct {
	Helper interfaces.HelperInterface
}

func (receiver *Env) Assembly() error {
	receiver.Helper.SetEnv(impl.NewEnv())

	return nil
}
