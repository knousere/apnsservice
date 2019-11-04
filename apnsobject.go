package apnsservice

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"

	apns "github.com/joekarl/go-libapns"

	"github.com/knousere/web-service-commons/utils"
)

type statusAPNS int

// apns status codes
const (
	ApnsUnknown statusAPNS = iota
	ApnsNoCerts
	ApnsCertsFound
	ApnsActive
)

// connectionAPNS is a structure for managing an APNS connection.
// It is internal to the apnsservice package.
type connectionAPNS struct {
	appID       int    // internal app identifier
	stringID    string // external app identifier
	fileLog     io.Writer
	loggers     map[int]*log.Logger
	cert        *AppCert
	cfgAPNS     *apns.APNSConfig
	cfgFeedback *apns.APNSFeedbackServiceConfig
	chanDone    chan struct{}
	chanDoneLog chan struct{}
	chanSend    chan *apns.Payload
	chanLog     chan *logEntry
	status      statusAPNS
	isLogging   bool
}

// logEntry is a structure for passing a formatted log message
// through the log channel.
type logEntry struct {
	socketID int
	message  string
}

// launch starts a pair of sockets for an apns object
// if certs are present. The sockets toggle to minimize blocking.
func (a *connectionAPNS) launch(isLogging bool) error {
	utils.Trace.Printf("launch %d, %s, %d", a.appID, a.stringID, int(a.status))

	var err error

	a.isLogging = isLogging

	switch a.status {
	case ApnsActive, ApnsNoCerts:
		return nil
	}

	a.cfgAPNS = &apns.APNSConfig{
		CertificateBytes: a.cert.Cert,
		KeyBytes:         a.cert.RSAKey,
		GatewayHost:      pushURL,
	}

	a.cfgFeedback = &apns.APNSFeedbackServiceConfig{
		CertificateBytes: a.cert.Cert,
		KeyBytes:         a.cert.RSAKey,
		GatewayHost:      feedbackURL,
	}

	strLogPath := fmt.Sprintf("logs/apns/%s.txt", a.stringID)
	a.fileLog, err = os.OpenFile(strLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		utils.Warning.Println("Error opening apns log ", strLogPath, err.Error())
		return err
	}
	feedbackLog := log.New(a.fileLog, "APN: ", log.Ldate|log.Ltime|log.Lshortfile)

	err = a.getBadTokens(feedbackLog)
	if err != nil {
		utils.Warning.Println("Error checking apns feedback ", a.stringID, err.Error())
		return err
	}

	a.chanDone = make(chan struct{})
	a.chanDoneLog = make(chan struct{})
	a.chanSend = make(chan *apns.Payload, 100)
	a.chanLog = make(chan *logEntry, 100)

	a.loggers = make(map[int]*log.Logger)

	for socketID := 1; socketID <= 2; socketID++ {
		strPrefix := fmt.Sprintf("APN%d: ", socketID)
		a.loggers[socketID] = log.New(a.fileLog, strPrefix, log.Ldate|log.Ltime|log.Lshortfile)
	}

	for socketID := 1; socketID <= 2; socketID++ {
		go a.launchSocket(socketID)
	}

	a.status = ApnsActive
	return nil
}

// Close shuts down the apns connection by closing the done channel
func (a *connectionAPNS) close() {
	if a.status == ApnsActive {
		close(a.chanDone)
		a.status = ApnsCertsFound
	}
}

// pushOne pushes one notification into the send channel.
func (a *connectionAPNS) pushOne(payload apns.Payload) {
	if a.status == ApnsActive { // safety first
		a.chanSend <- &payload
	}
}

// logPrint pushes a log entry.
func (a *connectionAPNS) logPrint(socketID int, args ...interface{}) {
	if a.isLogging {
		entry := logEntry{
			socketID: socketID,
		}
		entry.message = fmt.Sprint(args...)
		a.chanLog <- &entry
	}
}

// logPrint pushes a log entry terminated with line break.
func (a *connectionAPNS) logPrintln(socketID int, args ...interface{}) {
	if a.isLogging {
		entry := logEntry{
			socketID: socketID,
		}
		entry.message = fmt.Sprintln(args...)
		a.chanLog <- &entry
	}
}

// logPrint pushes a log entry with string formatting.
func (a *connectionAPNS) logPrintf(socketID int, format string, args ...interface{}) {
	if a.isLogging {
		entry := logEntry{
			socketID: socketID,
		}
		entry.message = fmt.Sprintf(format, args...)
		a.chanLog <- &entry
	}
}

// logListener listens on a.chanLog for entries from a socket
// and writes to the associated logger.
func (a *connectionAPNS) logListener() {
	bShutdown := false
	for {
		if bShutdown {
			break
		}
		select {
		case entry := <-a.chanLog:
			a.loggers[entry.socketID].Print(entry.message)
		case <-a.chanDoneLog:
			bShutdown = true
		}
	}
}

// launchSocket launches a channel listener.
// It pulls notifications from the send channel and pushes them through the apns socket
// until the either the send channel is empty or Apple closes the socket.
// The done channel shuts down this listner.
func (a *connectionAPNS) launchSocket(socketID int) {

	bShutdown := false
	bConnectionGood := false
	var connLast *apns.APNSConnection
	intCacheSize := int(32)
	intPayloadIdx := int(intCacheSize - 1)                           // index into cache
	payloadCache := make([]apns.Payload, intCacheSize, intCacheSize) // circular array of recent payloads
	exponentialBackoff := int(1)                                     // number of seconds between sending retries
	const backoffLimit = 128

	for { // loop until shutdown is declared
		if bShutdown {
			a.logPrintln(socketID, "Breaking the for loop, shutdown")
			break
		}

		a.logPrint(socketID, "Establishing connection")
		connAPNS, err := apns.NewAPNSConnection(a.cfgAPNS)

		if err == nil { // is connection good?
			connLast = connAPNS
			bConnectionGood = true
			a.logPrintln(socketID, "Connection established")
		} else {
			bConnectionGood = false
			a.logPrintf(socketID, " Error: %s\n", err.Error())

			select {
			case <-time.After(time.Second * 5):
				continue
			case <-a.chanDone:
				a.logPrintln(socketID, "Received done close")
				bShutdown = true
			}
		}

		for { // loop until connection is closed or shutdown is declared
			if !bConnectionGood || bShutdown {
				break
			}

			select { // either process a payload or handle the exception
			case payload := <-a.chanSend:
				a.logPrintf(socketID, "Push to device %v %s\n", payload.ExtraData, payload.AlertText)

				select {
				case <-time.After(time.Duration(exponentialBackoff) * time.Second):
					break
				case connAPNS.SendChannel <- payload: // send it and cache it
					intPayloadIdx = (intPayloadIdx + 1) % intCacheSize // increment mod 32
					payloadCache[intPayloadIdx] = *payload
					exponentialBackoff = 1
					break
				}
				break
			case closeError := <-connAPNS.CloseChannel:
				// Apple closed the connection and returned an error. This is usually due to INVALID_TOKEN or EOF.
				// Two most common reasons for EOF:
				// 1. Apple is verifying the socket. (every 2 hours)
				// 2. The connection was established with an incorrect cert. (EOF comes on every try.)
				a.logPrintln(socketID, "Received error, closing connection")
				if exponentialBackoff < backoffLimit {
					exponentialBackoff = exponentialBackoff * 2
				}
				a.handleCloseError(closeError, socketID, &payloadCache, intPayloadIdx)
				bConnectionGood = false
				break
			case <-a.chanDone:
				a.logPrintln(socketID, "Done channel is closed. Closing connection.")
				connAPNS.Disconnect()
				bShutdown = true
			}
		}
	}

	if connLast != nil {
		select {
		case <-time.After(time.Second * 5):
			a.logPrint(socketID, ".")
			break
		case closeError := <-connLast.CloseChannel:
			a.logPrintln(socketID, "Closing channel")
			a.handleCloseError(closeError, socketID, &payloadCache, intPayloadIdx)
		}
	}
	a.logPrintln(socketID, "Shutting down apns service")
	if bShutdown {
		close(a.chanDoneLog)
	}
}

// handleCloseError handles feedback after Apple closes the connection.
func (a *connectionAPNS) handleCloseError(closeError *apns.ConnectionClose, socketID int,
	cache *[]apns.Payload, intCurrentIdx int) {

	a.logPrintln(socketID, "CloseError: ", closeError.Error)
	intUnsentCount := closeError.UnsentPayloads.Len()
	// do something here with unsent payloads
	if intUnsentCount > 0 {
		a.logPrintf(socketID, "List length %d, Overflow %v\n",
			closeError.UnsentPayloads.Len(),
			closeError.UnsentPayloadBufferOverflow)
	}
	if closeError.ErrorPayload != nil {
		payload := closeError.ErrorPayload
		a.logPrintf(socketID, "Payload %v %s %s\n%s\n",
			payload.ExtraData,
			payload.Category,
			payload.AlertText,
			payload.Token)
	}

	if intUnsentCount > 0 {
		intCacheSize := cap(*cache)
		if intUnsentCount > intCacheSize {
			// prevent circular buffer overflow
			intUnsentCount = intCacheSize
		}
		for i := intUnsentCount; i > 0; i-- {
			intIdx := (intCurrentIdx + intCacheSize - i + 1) % intCacheSize
			payload := (*cache)[intIdx]
			a.PushOne(payload)
		}
	}
}

// getBadTokens gets list of recent bad tokens from Apple.
func (a *connectionAPNS) getBadTokens(apnLog *log.Logger) error {

	listResponse, err := apns.ConnectToFeedbackService(a.cfgFeedback)

	if err == nil {
		apnLog.Println("getBadTokens listResponse len", listResponse.Len())
		if listResponse.Len() > 0 {
			for e := listResponse.Front(); e != nil; e = e.Next() {
				feedback, ok := e.Value.(*apns.FeedbackResponse)
				if ok == true {
					ts := time.Unix(int64(feedback.Timestamp), 0)
					apnLog.Println("TimeStamp and Token", ts, feedback.Token)
				}
			}
		}
	} else {
		apnLog.Println("getBadTokens failed ", err.Error())
	}
	return err
}
