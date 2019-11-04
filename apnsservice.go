package apnsservice

import (
	apns "github.com/joekarl/go-libapns"
	"github.com/knousere/web-service-commons/utils"
)

// AppCert is a structure for passing RSA certificate associated with an App
type AppCert struct {
	AppID  int    `json:"appId"`
	IsDev  int    `json:"isDev"`
	Cert   []byte `json:"cert"`
	RSAKey []byte `json:"rsaKey"`
}

// mapAPNS stores all available APNS channels keyed by appID.
var mapAPNS map[int]*connectionAPNS

// These are Apple push notification parameters applied to all instances of connectionAPNS.
var pushURL string
var feedbackURL string

// InitMap initializes the APNS connection map.
// Run this once from main.
// This server is either production or development.
func InitMap(isDev int) {
	mapAPNS = make(map[int]*connectionAPNS)

	if isDev == 1 {
		pushURL = "gateway.sandbox.push.apple.com"
		feedbackURL = "feedback.sandbox.push.apple.com"
	} else {
		pushURL = "gateway.push.apple.com"
		feedbackURL = "feedback.push.apple.com"
	}
}

// LaunchConnection creates an initialized apns connection and adds it to the map
// if push is enabled for this app.
// Call this from main for each app.
func LaunchConnection(appID int, appString string, isPushEnabled int, appCert AppCert, isLogging bool) error {
	if isPushEnabled == 1 {
		connectionAPNS := newConnection(appID, appString, &appCert)
		err := connectionAPNS.launch(isLogging)
		if err != nil {
			utils.Warning.Println("connectionAPNS.launch()", appString, err.Error())
			return err
		}

		mapAPNS[appID] = &connectionAPNS
		utils.Info.Println(appString, " connection status=", connectionAPNS.status)
	}

	return nil
}

// newConnection returns an uninitialized connectionAPNS instance
func newConnection(appID int, stringID string, appCert *AppCert) connectionAPNS {
	status := ApnsNoCerts
	if appCert != nil {
		status = ApnsCertsFound
	}
	return connectionAPNS{
		appID:     appID,
		stringID:  stringID,
		status:    status,
		cert:      appCert,
		isLogging: true,
	}
}

// PushOne pushes one notification for the specified app.
func PushOne(appID int, payload apns.Payload) {
	connectionAPNS := mapAPNS[appID]
	if connectionAPNS != nil {
		connectionAPNS.pushOne(payload)
	}
}

// CloseConnection closes the apns connection for one app.
func CloseConnection(appID int) {
	connectionAPNS := mapAPNS[appID]
	if connectionAPNS != nil {
		connectionAPNS.close()
	}
}

// CloseAllConnections closes all apns connections.
// This is called at main shutdown
func CloseAllConnections() {
	for _, connectionAPNS := range mapAPNS {
		connectionAPNS.close()
	}
}
