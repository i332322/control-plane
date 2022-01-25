package main

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	reconcilerApi "github.com/kyma-incubator/reconciler/pkg/keb"
	"github.com/pivotal-cf/brokerapi/v8/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKymaUpgrade_UpgradeTo2(t *testing.T) {
	// given
	suite := NewBrokerSuiteTest(t)
	defer suite.TearDown()
	iid := uuid.New().String()

	// provision Kyma 1.x
	resp := suite.CallAPI("PUT", fmt.Sprintf("oauth/cf-eu10/v2/service_instances/%s?accepts_incomplete=true", iid),
		`{
					"service_id": "47c9dcbf-ff30-448e-ab36-d3bad66ba281",
					"plan_id": "4deee563-e5ec-4731-b9b1-53b42d855f0c",
					"context": {
						"sm_platform_credentials": {
							"url": "https://sm.url",
							"credentials": {
								"basic": {
									"username":"smUsername",
									"password":"smPassword"
							  	}
						}
							},
						"globalaccount_id": "g-account-id",
						"subaccount_id": "sub-id",
						"user_id": "john.smith@email.com"
					},
					"parameters": {
						"name": "testing-cluster"
					}
		}`)
	opID := suite.DecodeOperationID(resp)
	suite.processProvisioningByOperationID(opID)

	// when
	orchestrationResp := suite.CallAPI("POST", "upgrade/kyma",
		`{
				"strategy": {
				  "type": "parallel",
				  "schedule": "immediate",
				  "parallel": {
					"workers": 1
				  }
				},
				"dryRun": false,
				"targets": {
				  "include": [
					{
					  "subAccount": "sub-id"
					}
				  ]
				},
					"kyma": {
						"version": "2.0.0-rc4"
					}
				}`)
	oID := suite.DecodeOrchestrationID(orchestrationResp)

	suite.AssertReconcilerStartedReconcilingWhenUpgrading(iid)

	opResponse := suite.CallAPI("GET", fmt.Sprintf("orchestrations/%s/operations", oID), "")
	upgradeKymaOperationID, err := suite.DecodeLastUpgradeKymaOperationIDFromOrchestration(opResponse)
	require.NoError(t, err)

	suite.FinishUpgradeKymaOperationByReconciler(upgradeKymaOperationID)
	suite.AssertClusterKymaConfig(opID, reconcilerApi.KymaConfig{
		Version:        "2.0.0-rc4",
		Profile:        "Production",
		Administrators: []string{"john.smith@email.com"},
		Components:     suite.fixExpectedComponentListWithSMProxy(opID),
	})
	suite.AssertClusterConfigWithKubeconfig(opID)

	upgradeOp, err := suite.db.Operations().GetUpgradeKymaOperationByID(upgradeKymaOperationID)
	require.NoError(t, err)
	found := suite.provisionerClient.IsRuntimeUpgraded(upgradeOp.InstanceDetails.RuntimeID, "2.0.0-rc4")
	assert.False(t, found)
}

func TestKymaUpgrade_UpgradeAfterMigration(t *testing.T) {
	// given
	suite := NewBrokerSuiteTest(t)
	mockBTPOperatorClusterID()
	defer suite.TearDown()
	id := "InstanceID-UpgradeAfterMigration"

	// provision Kyma 2.0
	resp := suite.CallAPI("PUT", fmt.Sprintf("oauth/cf-eu10/v2/service_instances/%s?accepts_incomplete=true", id), `
{
	"service_id": "47c9dcbf-ff30-448e-ab36-d3bad66ba281",
	"plan_id": "4deee563-e5ec-4731-b9b1-53b42d855f0c",
	"context": {
		"sm_platform_credentials": {
			"url": "https://sm.url",
			"credentials": {
			"basic": {
					"username":"smUsername",
					"password":"smPassword"
	  			}
			}
		},
		"globalaccount_id": "g-account-id",
		"subaccount_id": "sub-id",
		"user_id": "john.smith@email.com"
	},
	"parameters": {
		"name": "testing-cluster",
		"kymaVersion": "2.0.0-rc4"
	}
}`)
	opID := suite.DecodeOperationID(resp)
	suite.processReconcilingByOperationID(opID)
	suite.WaitForOperationState(opID, domain.Succeeded)

	// migrate svcat
	resp = suite.CallAPI("PATCH", fmt.Sprintf("oauth/cf-eu10/v2/service_instances/%s?accepts_incomplete=true", id), `
{
	"service_id": "47c9dcbf-ff30-448e-ab36-d3bad66ba281",
	"context": {
		"globalaccount_id": "g-account-id",
		"subaccount_id": "sub-id",
		"user_id": "john.smith@email.com",
		"sm_operator_credentials": {
			"clientid": "testClientID",
			"clientsecret": "testClientSecret",
			"sm_url": "https://service-manager.kyma.com",
			"url": "https://test.auth.com",
			"xsappname": "testXsappname"
		},
		"isMigration": true
	}
}`)

	// make sure migration finished
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	updateOperationID := suite.DecodeOperationID(resp)
	suite.FinishUpdatingOperationByProvisioner(updateOperationID)
	suite.FinishUpdatingOperationByReconcilerBoth(updateOperationID)
	suite.WaitForOperationState(updateOperationID, domain.Succeeded)

	// ensure component list after update is correct
	i, err := suite.db.Instances().GetByID(id)
	assert.NoError(t, err, "getting instance after update")
	assert.True(t, i.InstanceDetails.SCMigrationTriggered, "instance SCMigrationTriggered after update")
	rsu1, err := suite.db.RuntimeStates().GetLatestWithReconcilerInputByRuntimeID(i.RuntimeID)
	assert.NoError(t, err, "getting runtime after update")
	assert.Equal(t, updateOperationID, rsu1.OperationID, "runtime state update operation ID")
	assert.ElementsMatch(t, componentNames(rsu1.ClusterSetup.KymaConfig.Components), []string{"ory", "monitoring", "btp-operator"})

	// run upgrade
	orchestrationResp := suite.CallAPI("POST", "upgrade/kyma", `
{
	"strategy": {
		"type": "parallel",
		"schedule": "immediate",
		"parallel": {
			"workers": 1
		}
	},
	"dryRun": false,
	"targets": {
		"include": [
			{
				"subAccount": "sub-id"
			}
		]
	},
	"kyma": {
		"version": "2.0.0"
	}
}`)
	oID := suite.DecodeOrchestrationID(orchestrationResp)
	suite.AssertReconcilerStartedReconcilingWhenUpgrading(id)
	opResponse := suite.CallAPI("GET", fmt.Sprintf("orchestrations/%s/operations", oID), "")
	upgradeKymaOperationID, err := suite.DecodeLastUpgradeKymaOperationIDFromOrchestration(opResponse)
	require.NoError(t, err)

	suite.FinishUpgradeKymaOperationByReconciler(upgradeKymaOperationID)
	suite.AssertClusterConfigWithKubeconfig(opID)

	_, err = suite.db.Operations().GetUpgradeKymaOperationByID(upgradeKymaOperationID)
	require.NoError(t, err)

	// ensure component list after upgrade didn't get changed
	i, err = suite.db.Instances().GetByID(id)
	assert.NoError(t, err, "getting instance after upgrade")
	assert.True(t, i.InstanceDetails.SCMigrationTriggered, "instance SCMigrationTriggered after upgrade")
	rsu2, err := suite.db.RuntimeStates().GetLatestWithReconcilerInputByRuntimeID(i.RuntimeID)
	assert.NoError(t, err, "getting runtime after upgrade")
	assert.NotEqual(t, rsu1.ID, rsu2.ID, "runtime_state ID from update should differ runtime_state ID from upgrade")
	assert.Equal(t, upgradeKymaOperationID, rsu2.OperationID, "runtime state upgrade operation ID")
	assert.ElementsMatch(t, componentNames(rsu2.ClusterSetup.KymaConfig.Components), []string{"ory", "monitoring", "btp-operator"})
}