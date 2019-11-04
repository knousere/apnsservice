package apnsservice

// This source code includes the exposed elements of the apns service. It is designed
// to be called from main or any api handler that uses push notifications.

import (
	apns "github.com/joekarl/go-libapns"
	"github.com/knousere/web-service-commons/utils"
)

// AppCert is a structure for passing RSA certificate associated with an App.
// If IsDev is non-zero then the cert is only valid for sandbox connections.
type AppCert struct {
	AppID  int    `json:"appId"`
	IsDev  int    `json:"isDev"`
	Cert   []byte `json:"cert"`
	RSAKey []byte `json:"rsaKey"`
}

// mapAPNS stores all available APNS channels keyed by appID.
var mapAPNS map[int]*connectionAPNS

func init() {
	mapAPNS = make(map[int]*connectionAPNS)
}

// These are Apple push notification URLs applied to all instances of connectionAPNS.
var pushURL string
var feedbackURL string

// InitURLs initializes the APNS gateway URLs.
// Run this once from main before launching any connections.
// This server is either production or development.
func InitURLs(isDev bool) {
	if isDev {
		pushURL = "gateway.sandbox.push.apple.com"
		feedbackURL = "feedback.sandbox.push.apple.com"
	} else {
		pushURL = "gateway.push.apple.com"
		feedbackURL = "feedback.push.apple.com"
	}
}

// LaunchConnection creates an initialized apns connection
// and adds it to the map if push is enabled for this app.
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

// newConnection returns a connectionAPNS instance
func newConnection(appID int, stringID string, appCert *AppCert) connectionAPNS {
	status := apnsNoCerts
	if appCert != nil {
		status = apnsCertsFound
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
// This is called at main shutdown.
func CloseAllConnections() {
	for _, connectionAPNS := range mapAPNS {
		connectionAPNS.close()
	}
}
