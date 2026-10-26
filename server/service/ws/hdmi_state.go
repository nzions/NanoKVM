package ws

import (
	"encoding/json"

	"NanoKVM-Server/service/hdmi"

	log "github.com/sirupsen/logrus"
)

// HdmiStateEvent carries an hdmi.Snapshot, pushed on every HDMI transition.
const HdmiStateEvent = "hdmi-state"

func init() {
	sub := hdmi.Default().Subscribe()
	go func() {
		for snapshot := range sub.C {
			broadcastHdmiState(snapshot)
		}
	}()
}

func sendHdmiStateSnapshot(client *Client) {
	snapshot, ok := hdmi.Default().Current()
	if !ok {
		return
	}
	if err := sendHdmiState(client, snapshot); err != nil {
		log.Errorf("failed to send hdmi state snapshot: %s", err)
	}
}

func broadcastHdmiState(snapshot hdmi.Snapshot) {
	for _, client := range GetManager().GetClients() {
		if err := sendHdmiState(client, snapshot); err != nil {
			log.Errorf("failed to send hdmi state: %s", err)
		}
	}
}

func sendHdmiState(client *Client, snapshot hdmi.Snapshot) error {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	return client.Write(HdmiStateEvent, string(payload))
}
