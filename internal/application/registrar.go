package application

import (
	"time"

	"github.com/vfeitoza/cli-smart-router/internal/domain"
	"github.com/vfeitoza/cli-smart-router/internal/infrastructure"
)

const providerName = "smart-model-router"

// Registrar handles virtual model registration.
type Registrar struct {
	Config domain.Config
}

// Register returns the configured virtual model metadata.
func (r Registrar) Register() infrastructure.ModelRegistrationResponse {
	cfg := r.Config.Normalize()
	return infrastructure.ModelRegistrationResponse{
		Provider: providerName,
		Models: []infrastructure.ModelInfo{{
			ID:                         cfg.VirtualModel,
			Object:                     "model",
			Created:                    time.Now().Unix(),
			OwnedBy:                    providerName,
			DisplayName:                "Smart Model Router",
			Description:                "Virtual model routed by smart-model-router.",
			SupportedGenerationMethods: []string{"chat"},
			UserDefined:                true,
		}},
	}
}
