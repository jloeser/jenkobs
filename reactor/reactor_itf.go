package jenkobs_reactor

import (
	"encoding/json"

	wzlib_logger "github.com/infra-whizz/wzlib/logger"
	"github.com/streadway/amqp"
)

// ReactorAction interface
type ReactorAction interface {
	GetActionInfo() *ActionInfo
	MakeActionInstance() interface{}
	LoadAction(action *ActionInfo)
	OnMessage(message *ReactorDelivery) error
}

// BaseAction object
type BaseAction struct {
	actionInfo *ActionInfo
	wzlib_logger.WzLogger
}

// Matches the criteria in the actions configuration
func (bsa *BaseAction) Matches(message *ReactorDelivery) bool {
	// Matches status
	if message.GetDelivery().RoutingKey != bsa.actionInfo.Status {
		return false
	}

	// Matches Project. To match any projects, use "*"
	if bsa.actionInfo.Project != "*" && message.GetProjectName() != bsa.actionInfo.Project {
		return false
	}

	// Matches Architecture (if defined)
	if bsa.actionInfo.Architecture != "" && bsa.actionInfo.Architecture != message.GetArch() {
		return false
	}

	// Matches Package (if defined)
	if bsa.actionInfo.Package != "" && bsa.actionInfo.Package != message.GetPackageName() {
		return false
	}
	return true
}

// GetActionInfo on request for this action object
func (bsa *BaseAction) GetActionInfo() *ActionInfo {
	return bsa.actionInfo
}

// LoadAction info
func (bsa *BaseAction) LoadAction(action *ActionInfo) {
	bsa.actionInfo = action
}

// MakeActionInstance creates a self-contained instance copy
func (bsa *BaseAction) MakeActionInstance() interface{} {
	panic("Make Action Instance is not implemented yet")
}

// ReactorDelivery object
type ReactorDelivery struct {
	content  map[string]interface{}
	delivery *amqp.Delivery
	wzlib_logger.WzLogger
}

// NewReactorDelivery constructor for ReactorDelivery object
func NewReactorDelivery(delivery *amqp.Delivery) *ReactorDelivery {
	rd := new(ReactorDelivery)
	rd.delivery = delivery
	if err := json.Unmarshal(rd.delivery.Body, &rd.content); err != nil {
		rd.GetLogger().Debugf("ERROR: Delivery object is a bad or broken JSON: %s", err.Error())
		rd.GetLogger().Debugf("Content: %s", rd.delivery.Body)
	}

	return rd
}

// IsValid returns true if JSON content was parsed
func (rd *ReactorDelivery) IsValid() bool {
	return rd.content != nil
}

// GetContent of the JSON body, parsed
func (rd *ReactorDelivery) GetContent() map[string]interface{} {
	return rd.content
}

// GetDelivery object from the AMQP
func (rd *ReactorDelivery) GetDelivery() *amqp.Delivery {
	return rd.delivery
}

// GetStatus returns delivery type, if any. Example: "opensuse.obs.repo.published".
func (rd *ReactorDelivery) GetStatus() string {
	return rd.delivery.Type
}

// Get string content, if any. If none or the whole Body is invalid, return an empty string.
func (rd *ReactorDelivery) getContentString(key string) string {
	if rd.IsValid() {
		proj, ex := rd.content[key]
		if ex {
			v, ok := proj.(string)
			if ok {
				return v
			}
		}
	}
	return ""
}

// GetPackageName to the entity those status is applicable.
// If Delivery is not valid, an empty string returned.
func (rd *ReactorDelivery) GetPackageName() string {
	return rd.getContentString("package")
}

// GetProjectName of the repo. If Delivery is not valid, an empty string returned.
func (rd *ReactorDelivery) GetProjectName() string {
	return rd.getContentString("project")
}

// GetArch (architecture) of the package/repo. If Delivery is not valid, an empty string returned.
func (rd *ReactorDelivery) GetArch() string {
	return rd.getContentString("arch")
}

// GetRepoName of the project's repository. If Delivery is not valid, an empty string returned.
func (rd *ReactorDelivery) GetRepoName() string {
	repo := rd.getContentString("repo")
	if repo == "" {
		repo = rd.getContentString("repository")
	}
	return repo
}
