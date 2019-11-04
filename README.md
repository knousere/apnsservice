# apnsservice
A non-blocking implementation of Apple Push Notification Service (apns1).

APNS comes in two versions. APNS-1 uses an Apple proprietary streaming protocol. Push notifications are sent in a continuous stream without acknowledgement until an exception is encountered. At that point, the APNS gateway closes the socket and responds with an error message and a count of how many push notifications need to be resent. Programmers apparently struggled to implement a proper non-blocking interface to this protocol so Apple introduced APNS-2 which uses http protocol which is slower but surer.

This code sample represents a service that is internal to a REST server. It shows off a number of Golang features that minimize blocking. the code sample passes lint but has not been tested live. It was derived from an existing production server that I wrote that supports multiple cellphone client apps that talk to portions of the one REST API. This production server has been in continuous production using simlar code for over four years.

For each supported app a pair of sockets to the APNS gateway sends on a toggling basis. So if one is blocked, the other immediately takes over. Only one is sending at any given time. To send a push notification, the PushOne function is called as a go routine. The push notification is sent through a channel to be queued into a circular cache. The active connection pulls from the circular cache and sends to the APNS gateway. If an error response is received, the cursor is reset to the element just after the element that triggered the error. The size of the circular cache is set to be larger than the expected maximum resend count. The code truncates the retry count to prevent underflowing the cache.

Currently, no attempt is made to fix the offending element. A push notification usually fails due to an expired token associated with a particular cellphone device. The remedy is usually to update the token or inactivate a record from the client database.

There are also cases where the gateway goes down or the connection is periodically reset by the host. The caller implements exponential backoff in order not to overload the gateway.

Diagnostic messages are pushed to a log channel. A channel listener pulls from the log channel and calls the appropriate function of the golang standard log package. Logging is per app. Wrappers are supported for Print, Println and Printf. The channel listener provides a convenient point of interface to an external logging service if such is desired.
