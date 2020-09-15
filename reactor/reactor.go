package jenkobs_reactor

import (
	"fmt"
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v2"

	wzlib_logger "github.com/infra-whizz/wzlib/logger"
	"github.com/streadway/amqp"
)

type Reactor struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	queue   amqp.Queue
	actions []ReactorAction

	user     string
	password string
	fqdn     string
	port     int

	wzlib_logger.WzLogger
}

func NewReactor() *Reactor {
	rtr := new(Reactor)
	rtr.actions = make([]ReactorAction, 0)
	return rtr
}

// SetAMQPDial string
func (rtr *Reactor) SetAMQPDial(user string, password string, fqdn string, port int) *Reactor {
	rtr.user = user
	rtr.password = password
	rtr.fqdn = fqdn
	rtr.port = port
	return rtr
}

func (rtr *Reactor) connectAMQP() error {
	if rtr.user == "" || rtr.fqdn == "" {
		err := fmt.Errorf("Error connecting to the AMQP server: user or FQDN are missing")
		rtr.GetLogger().Error(err.Error())
		return err
	}
	var err error
	var connstr string
	if rtr.port > 0 {
		connstr = fmt.Sprintf("amqps://%s:%s@%s:%d/", rtr.user, rtr.password, rtr.fqdn, rtr.port)
	} else {
		connstr = fmt.Sprintf("amqps://%s:%s@%s/", rtr.user, rtr.password, rtr.fqdn)
	}
	rtr.conn, err = amqp.Dial(connstr)
	if err != nil {
		rtr.GetLogger().Errorf("Error connecting to the AMQP server: %s", err.Error())
		return err
	} else {
		rtr.GetLogger().Infof("Connected to AMQP at %s", rtr.fqdn)
	}

	// Setup channel
	rtr.channel, err = rtr.conn.Channel()
	err = rtr.channel.ExchangeDeclarePassive("pubsub", "topic", true, false, false, false, nil)
	if err != nil {
		rtr.GetLogger().Errorf("Error creating AMQP channel: %s", err.Error())
		return err
	} else {
		rtr.GetLogger().Infof("Created AMQP channel")
	}

	// Setup queue
	rtr.queue, err = rtr.channel.QueueDeclare("", false, false, true, false, nil)
	if err != nil {
		rtr.GetLogger().Errorf("Error setting up queue: %s", err.Error())
		return err
	} else {
		rtr.GetLogger().Infof("Default queue declared")
	}

	if err = rtr.channel.QueueBind(rtr.queue.Name, "#", "pubsub", false, nil); err != nil {
		rtr.GetLogger().Errorf("Error binding queue '%s' to the channel: %s", rtr.queue.Name, err.Error())
		return err
	} else {
		rtr.GetLogger().Infof("Bound queue '%s' to the channel", rtr.queue.Name)
	}

	return nil
}

func (rtr *Reactor) getAction(actionSet map[string]interface{}) *ActionInfo {
	// Always only one element anyways
	for key, data := range actionSet {
		actionData := data.(map[interface{}]interface{})
		params := make(map[string]interface{})

		var actionType string
		for pkey, pval := range actionData["action"].(map[interface{}]interface{}) {
			spkey := pkey.(string)
			if spkey == "type" {
				actionType = pval.(string)
			} else {
				switch actionType {
				case "ci":
					// Params are key/value with nested key/value for query params
					paramsItf, ok := pval.(map[interface{}]interface{})
					if ok {
						paramsSet := make(map[string]interface{})
						for cmdKey, cmdVal := range paramsItf {
							if cmdKey.(string) == "params" {
								queryParams := make(map[string]interface{})
								if cmdVal != nil {
									for qp, qv := range cmdVal.(map[interface{}]interface{}) {
										queryParams[qp.(string)] = qv
									}
								}
								paramsSet["params"] = queryParams

							} else {
								paramsSet[cmdKey.(string)] = cmdVal
							}
						}
						params[spkey] = paramsSet
					} else {
						rtr.GetLogger().Error("HTTP action does not have a proper caller configuration")
					}
				case "shell":
					// Params are string array just like a typical command line
					cmdKeysItf, ok := pval.([]interface{})
					if ok {
						cmdParams := make([]string, 0)
						for _, cmdParam := range cmdKeysItf {
							cmdParams = append(cmdParams, cmdParam.(string))
						}
						params[spkey] = cmdParams
					} else {
						rtr.GetLogger().Error("Command should be an array of strings in the action definition!")
						return nil
					}
				case "":
				default:
					rtr.GetLogger().Errorf("Unsupported action type: %s", actionType)
				}
			}
		}

		var packageName string
		var archName string
		var statusName string

		if actionData["package"] != nil {
			packageName = actionData["package"].(string)
		}

		if actionData["arch"] != nil {
			archName = actionData["arch"].(string)
		}

		if actionData["status"] != nil {
			statusName = actionData["status"].(string)
		} else {
			rtr.GetLogger().Warnf("Action on project '%s' has no defined status, skipping", key)
			return nil
		}

		action := &ActionInfo{
			Project:      key,
			Package:      packageName,
			Status:       statusName,
			Architecture: archName,
			Params:       params,
			Type:         actionType,
		}
		if actionType == "" {
			rtr.GetLogger().Warnf("Action on project '%s' with package '%s' does not have defined action type, skipping",
				action.Project, action.Package)
			return nil
		} else {
			return action
		}
	}
	return nil
}

// LoadConfig of the reactor
func (rtr *Reactor) LoadActions(actionsCfgPath string) *Reactor {
	content, err := ioutil.ReadFile(actionsCfgPath)
	if err != nil {
		rtr.GetLogger().Errorf("Unable to load actions: %s", err.Error())
		os.Exit(1)
	}

	var data []map[string]interface{}
	if err := yaml.Unmarshal(content, &data); err != nil {
		rtr.GetLogger().Errorf("Unable to read actions configuration: %s", err.Error())
	}
	var loaded int
	for _, actionSet := range data {
		action := rtr.getAction(actionSet)
		if action != nil {
			switch action.Type {
			case ACTION_TYPE_CI:
				httpAction := NewHTTPAction()
				httpAction.LoadAction(action)
				rtr.actions = append(rtr.actions, httpAction)
				loaded++
				rtr.GetLogger().Debugf("Loaded criteria HTTP matcher for project '%s'", action.Project)
			case ACTION_TYPE_SHELL:
				shellAction := NewShellAction()
				shellAction.LoadAction(action)
				rtr.actions = append(rtr.actions, shellAction)
				loaded++
				rtr.GetLogger().Debugf("Loaded criteria Shell matcher for project '%s'", action.Project)
			default:
				rtr.GetLogger().Errorf("Unknown action type: %s for action %s", action.Type, action.Project)
			}
		}
	}
	rtr.GetLogger().Infof("Loaded %d matchers", loaded)
	return rtr
}

func (rtr *Reactor) onDelivery(delivery amqp.Delivery) error {
	rd := NewReactorDelivery(&delivery)
	if rd.IsValid() { // Some messages from OBS simply aren't JSON. :-(
		for _, action := range rtr.actions {
			go action.OnMessage(rd)
		}
	}
	return nil
}

func (rtr *Reactor) consume() error {
	msgChannel, err := rtr.channel.Consume(rtr.queue.Name, "", true, false, false, false, nil)
	if err != nil {
		return err
	}
	looper := make(chan bool)

	go func() {
		rtr.GetLogger().Debug("Listening to the events...")
		for delivery := range msgChannel {
			go rtr.onDelivery(delivery)
		}
	}()

	<-looper
	return nil
}

// Run the reactor
func (rtr *Reactor) Run() error {
	if err := rtr.connectAMQP(); err == nil {
		defer rtr.conn.Close()
		if err := rtr.consume(); err != nil {
			rtr.GetLogger().Errorf("Error consuming messages: %s", err.Error())
		}
	}

	return nil
}
