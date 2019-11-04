# apnsservice
A non-blocking implementation of Apple Push Notification Service (Apple Binary Protocol).
It acts as an implementation wrapper for Joe Karl's library [apns](http://github.com/joekarl/go-libapns) package.

## Overview
APNS comes in two versions. [Apple Binary Protocol](https://developer.apple.com/library/archive/documentation/NetworkingInternet/Conceptual/RemoteNotificationsPG/BinaryProviderAPI.html#//apple_ref/doc/uid/TP40008194-CH13-SW1) uses an Apple proprietary streaming protocol. Push notifications are sent in a continuous stream without acknowledgement until an exception is encountered. At that point, the APNS gateway closes the socket and responds with an error message and a count of how many push notifications need to be resent. Programmers apparently struggled to implement a proper non-blocking interface to this protocol so Apple introduced APNS/2 which uses HTTP/2.

This code sample represents a service that is internal to a REST server. It shows off a number of Golang features that minimize blocking. This code sample passes lint but has not been tested live. It was derived from an existing production server that I wrote that supports multiple cellphone client apps that talk to portions of a shared REST API. This production server has been in continuous production using simlar code for over four years.

For each supported app a pair of sockets to the APNS gateway sends payloads on a toggling basis. If one is blocked, the other immediately takes over. Only one is sending at any given time. Call the PushOne function as a go routine to send a push notification. The push notification is then pushed through the send channel for the calling app. The active connection for that app connection pulls an element from the send channel, sends it through the APNS gateway socket and also adds it to a circular cache.

If an error response is received, the cache cursor is reset to the cache element just after the element that triggered the error. All elements to be resent are pushed to the send channel. The size of the circular cache is set to be larger than the expected maximum resend count. The code truncates the resend count to prevent underflowing the cache. The cache is shared between the socket pair so whichever socket is active can pull from the cache and resend.

Currently, no attempt is made to fix the offending element. A push notification usually fails due to an expired token associated with a particular cellphone device. The remedy is usually to update the token or inactivate a record from the client database in an off-line process.

There are also cases where the gateway goes down or the connection is periodically reset by the host. The caller implements exponential backoff in order not to overload the gateway.

Diagnostic messages are pushed to a log channel. A channel listener pulls from the log channel and calls the appropriate function of the golang standard log package. Logging is per app. Wrappers are supported for Print, Println and Printf. The channel listener provides a convenient point of interface to an external logging service if such is desired.

## Usage
All exposed code is in apnsservice.go.
```go
import (
	"github.com/knousere/apnsservice"
)
```

### Before launching any connections, call this from main.
```go
apnsservice.InitURLs(true)
```

### A structure for passing RSA certificates to apnsservice
```go
// AppCert is a structure for passing the RSA certificate associated with an App.
// If IsDev is non-zero then the cert is only valid for sandbox connections.
type AppCert struct {
  AppID  int    `json:"appId"`
  IsDev  int    `json:"isDev"`
  Cert   []byte `json:"cert"`
  RSAKey []byte `json:"rsaKey"`
}
```

### Launch connections
Typically main would loop through initialization for each app. App Initialization parameters would typically be read from a database. If an app has push notifications enabled and has valid RSA certs then a connection would be launched.
```go
for _, p := range appParams {
  if p.IsPushEnabled {
    appCert, err := getAppCert(p.AppID) // from the database
    if err != nil {
      err = apnsservice.LaunchConnection(p.AppId, p.StringID, true, appCert, true)
      if err != nil {
        // handle err
      }
    }else{
      // handle err
    }
  }
}
```

### Send a push notification
This would be called within an api handler that would know the appID, userID and message from the http request.
```go
token, err = getDeviceToken(userID) // from the database
if err != nil {
  // handle err
}
payload := apns.Payload {
  Token: token,
  AlertText: message,
}
go apnsservice.PushOne(appID, payload)
```

### Close a connection
This ensures that send buffers are cleared and the connection is closed cleanly.
After closing a connection it is possible to call LaunchConnection again.
```go
apnsservice.CloseConnection(appID)
```

### Close all connections
Call this from main at server shutdown.
```go
apnsservice.CloseAllConnections()
```
