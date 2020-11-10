package upgrade_kyma

import (
	"testing"
	"time"

	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/process/upgrade_kyma/automock"
	"github.com/kyma-project/control-plane/components/kyma-environment-broker/internal/storage"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestOverridesFromSecretsAndConfigStep_Run_WithVersionComputed(t *testing.T) {
	t.Run("success run", func(t *testing.T) {
		// Given
		planName := "gcp"
		kymaVersion := "1.15.0"

		memoryStorage := storage.NewMemoryStorage()

		inputCreatorMock := &automock.ProvisionerInputCreator{}
		defer inputCreatorMock.AssertExpectations(t)

		runtimeOverridesMock := &automock.RuntimeOverridesAppender{}
		defer runtimeOverridesMock.AssertExpectations(t)
		runtimeOverridesMock.On("Append", inputCreatorMock, planName, kymaVersion).Return(nil).Once()

		operation := internal.UpgradeKymaOperation{
			InputCreator:           inputCreatorMock,
			ProvisioningParameters: `{ "plan_id": "ca6e5357-707f-4565-bbbd-b3ab732597c6" }`,
		}

		rvcMock := &automock.RuntimeVersionConfiguratorForUpgrade{}
		defer rvcMock.AssertExpectations(t)
		rvcMock.On("ForUpgrade").Return(&internal.RuntimeVersionData{Version: kymaVersion}).Once()

		step := NewOverridesFromSecretsAndConfigStep(memoryStorage.Operations(), runtimeOverridesMock, rvcMock)

		// When
		operation, repeat, err := step.Run(operation, logrus.New())

		// Then
		assert.NoError(t, err)
		assert.Equal(t, time.Duration(0), repeat)
	})
}

func TestOverridesFromSecretsAndConfigStep_Run_WithVersionFromOperation(t *testing.T) {
	t.Run("success run", func(t *testing.T) {
		// Given
		planName := "gcp"
		kymaVersion := "1.15.0"

		memoryStorage := storage.NewMemoryStorage()

		inputCreatorMock := &automock.ProvisionerInputCreator{}
		defer inputCreatorMock.AssertExpectations(t)

		runtimeOverridesMock := &automock.RuntimeOverridesAppender{}
		defer runtimeOverridesMock.AssertExpectations(t)
		runtimeOverridesMock.On("Append", inputCreatorMock, planName, kymaVersion).Return(nil).Once()

		operation := internal.UpgradeKymaOperation{
			InputCreator:           inputCreatorMock,
			ProvisioningParameters: `{ "plan_id": "ca6e5357-707f-4565-bbbd-b3ab732597c6" }`,
			RuntimeVersion: internal.RuntimeVersionData{
				Version: kymaVersion,
			},
		}

		rvcMock := &automock.RuntimeVersionConfiguratorForUpgrade{}
		defer rvcMock.AssertExpectations(t)

		step := NewOverridesFromSecretsAndConfigStep(memoryStorage.Operations(), runtimeOverridesMock, rvcMock)

		// When
		operation, repeat, err := step.Run(operation, logrus.New())

		// Then
		assert.NoError(t, err)
		assert.Equal(t, time.Duration(0), repeat)
	})
}