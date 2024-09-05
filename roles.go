package main

import (
	"strings"

	cb "github.com/clearblade/Go-SDK"
	cbiotcore "github.com/clearblade/go-iot"
)

const topicToken = "{device_id}"

var subTopics = [3]string{"/devices/" + topicToken + "/commands/#", "/devices/" + topicToken + "/config", "/devices/" + topicToken + "/errors"}
var pubTopics = [2]string{"/devices/" + topicToken + "/events/#", "/devices/" + topicToken + "/state"}

func createRoleForDevice(resultC chan ErrorLog, device *cbiotcore.Device) (map[string]interface{}, error) {
	role, err := cbDevClient.CreateRole(Args.cbSystemKey, device.Id)
	if err != nil {
		// Checking if role exists
		if !strings.Contains(err.Error(), "A role's name must be unique") {
			resultC <- ErrorLog{
				DeviceId: device.Id,
				Context:  "Error when Creating role",
				Error:    err,
			}
		} else {
			//Retrieve the role and return it
			role, err = cbDevClient.GetRole(Args.cbSystemKey, device.Id)
			if err != nil {
				resultC <- ErrorLog{
					DeviceId: device.Id,
					Context:  "Error when retrieving role",
					Error:    err,
				}
			}
		}
	}
	return role.(map[string]interface{}), err
}

func addTopicsToRole(resultC chan ErrorLog, device *cbiotcore.Device, roleId string) error {
	var err error
	//Add permissions for the subscribe topics
	for _, topic := range subTopics {
		err = cbDevClient.AddTopicToRole(Args.cbSystemKey, strings.Replace(topic, topicToken, device.Id, -1), roleId, cb.PERM_READ)
		if err != nil {
			resultC <- ErrorLog{
				DeviceId: device.Id,
				Context:  "Error when adding topic to role",
				Error:    err,
			}
			return err
		}
	}

	//Add permissions for the publish topics
	for _, topic := range pubTopics {
		err = cbDevClient.AddTopicToRole(Args.cbSystemKey, strings.Replace(topic, topicToken, device.Id, -1), roleId, cb.PERM_CREATE)
		if err != nil {
			resultC <- ErrorLog{
				DeviceId: device.Id,
				Context:  "Error when adding topic to role",
				Error:    err,
			}
			return err
		}
	}
	return err
}

func addDeviceToRole(resultC chan ErrorLog, device *cbiotcore.Device) error {
	err := cbDevClient.AddDeviceToRoles(Args.cbSystemKey, device.Id, []string{device.Id})
	if err != nil {

		if !strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			resultC <- ErrorLog{
				DeviceId: device.Id,
				Context:  "Error when Creating role",
				Error:    err,
			}
		} else {
			err = nil
		}
	}
	return err
}
