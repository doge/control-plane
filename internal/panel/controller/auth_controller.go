package controller

import (
	"context"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/example/control-plane/internal/models"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// setupStatus reports whether the first root account still needs to be created.
func (c *Controller) setupStatus(w http.ResponseWriter, r *http.Request) {
	n, e := c.service.Users.CountDocuments(r.Context(), bson.D{})
	if e != nil {
		writeJSON(w, 500, map[string]string{"error": e.Error()})
		return
	}
	writeJSON(w, 200, map[string]bool{"setupRequired": n == 0})
}

// setup begins first-user enrollment and returns authenticator setup details.
func (c *Controller) setup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	n, _ := c.service.Users.CountDocuments(r.Context(), bson.D{})
	if n > 0 {
		http.NotFound(w, r)
		return
	}
	var in struct{ Username, Email, Password string }
	if bodyJSON(r, &in) != nil || len(in.Username) < 3 || len(in.Password) < 8 || in.Email == "" {
		writeJSON(w, 400, map[string]string{"error": "username, email and password (8+ chars) are required"})
		return
	}
	h, err := hashPassword(in.Password)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "password hashing failed"})
		return
	}
	secret, err := newTOTPSecret()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not initialize authenticator"})
		return
	}
	id := randomToken(24)
	pending := bson.M{"_id": id, "username": strings.TrimSpace(in.Username), "email": strings.TrimSpace(in.Email), "passwordHash": h, "totpSecret": secret, "expiresAt": time.Now().Add(15 * time.Minute)}
	if _, err = c.service.SetupEnrollments.ReplaceOne(r.Context(), bson.M{"_id": id}, pending, true); err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not save enrollment"})
		return
	}
	writeJSON(w, 201, map[string]any{"setupId": id, "secret": secret, "otpauthUrl": totpURL(in.Email, secret)})
}

// setupVerify validates authenticator enrollment before creating the root user.
func (c *Controller) setupVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var in struct {
		SetupID string `json:"setupId"`
		Code    string `json:"code"`
	}
	if bodyJSON(r, &in) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	var pending struct {
		ID           string    `bson:"_id"`
		Username     string    `bson:"username"`
		Email        string    `bson:"email"`
		PasswordHash string    `bson:"passwordHash"`
		Secret       string    `bson:"totpSecret"`
		ExpiresAt    time.Time `bson:"expiresAt"`
	}
	if c.service.SetupEnrollments.FindOne(r.Context(), bson.M{"_id": in.SetupID, "expiresAt": bson.M{"$gt": time.Now()}}).Decode(&pending) != nil || !verifyTOTP(pending.Secret, in.Code, time.Now()) {
		writeJSON(w, 401, map[string]string{"error": "invalid or expired authenticator code"})
		return
	}
	count, err := c.service.Users.CountDocuments(r.Context(), bson.D{})
	if err != nil || count != 0 {
		writeJSON(w, 409, map[string]string{"error": "setup already completed"})
		return
	}
	u := models.User{Username: pending.Username, Email: pending.Email, PasswordHash: pending.PasswordHash, Role: "root", TOTPSecret: pending.Secret, TOTPEnabled: true, CreatedAt: time.Now()}
	recoveryCodes, recoveryHashes, err := generateTOTPRecoveryCodes()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not generate authenticator recovery codes"})
		return
	}
	u.TOTPRecoveryHashes = recoveryHashes
	if _, err = c.service.Users.InsertOne(r.Context(), u); err != nil {
		writeJSON(w, 409, map[string]string{"error": "setup already completed"})
		return
	}
	_, _ = c.service.SetupEnrollments.DeleteOne(r.Context(), bson.M{"_id": in.SetupID})
	writeJSON(w, 201, map[string]any{"created": true, "recoveryCodes": recoveryCodes})
}

// login authenticates a user and starts a second-factor challenge when enabled.
func (c *Controller) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var in struct{ Username, Password string }
	if bodyJSON(r, &in) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	var u models.User
	if c.service.Users.FindOne(r.Context(), bson.M{"$or": []bson.M{{"username": in.Username}, {"email": in.Username}}}).Decode(&u) != nil || u.Disabled || !verifyPassword(in.Password, u.PasswordHash) {
		writeJSON(w, 401, map[string]string{"error": "invalid credentials"})
		return
	}
	if u.TOTPEnabled {
		challenge := randomToken(24)
		_, err := c.service.LoginChallenges.InsertOne(r.Context(), bson.M{"_id": challenge, "userId": u.ID, "expiresAt": time.Now().Add(5 * time.Minute), "attempts": 0})
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not start verification"})
			return
		}
		writeJSON(w, 200, map[string]any{"requiresOTP": true, "challengeId": challenge})
		return
	}
	c.issueSession(w, r, u)
}

// loginOTP completes a pending login using an authenticator or recovery code.
func (c *Controller) loginOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var in struct {
		ChallengeID string `json:"challengeId"`
		Code        string `json:"code"`
	}
	if bodyJSON(r, &in) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	var challenge struct {
		UserID   bson.ObjectID `bson:"userId"`
		Attempts int           `bson:"attempts"`
	}
	filter := bson.M{"_id": in.ChallengeID, "expiresAt": bson.M{"$gt": time.Now()}, "attempts": bson.M{"$lt": 5}}
	if c.service.LoginChallenges.FindOne(r.Context(), filter).Decode(&challenge) != nil {
		writeJSON(w, 401, map[string]string{"error": "verification expired; sign in again"})
		return
	}
	var user models.User
	if c.service.Users.FindOne(r.Context(), bson.M{"_id": challenge.UserID, "disabled": bson.M{"$ne": true}}).Decode(&user) != nil || !user.TOTPEnabled {
		_, _ = c.service.LoginChallenges.UpdateOne(r.Context(), bson.M{"_id": in.ChallengeID}, bson.M{"$inc": bson.M{"attempts": 1}})
		writeJSON(w, 401, map[string]string{"error": "invalid authenticator code"})
		return
	}
	validCode := verifyTOTP(user.TOTPSecret, in.Code, time.Now())
	if !validCode {
		validCode = c.consumeTOTPRecoveryCode(r.Context(), user.ID, in.Code)
	}
	if !validCode {
		_, _ = c.service.LoginChallenges.UpdateOne(r.Context(), bson.M{"_id": in.ChallengeID}, bson.M{"$inc": bson.M{"attempts": 1}})
		writeJSON(w, 401, map[string]string{"error": "invalid authenticator or recovery code"})
		return
	}
	_, _ = c.service.LoginChallenges.DeleteOne(r.Context(), bson.M{"_id": in.ChallengeID})
	c.issueSession(w, r, user)
}

// consumeTOTPRecoveryCode atomically consumes one unused recovery code.
func (c *Controller) consumeTOTPRecoveryCode(ctx context.Context, userID bson.ObjectID, code string) bool {
	hash := recoveryCodeHash(code)
	if hash == "" {
		return false
	}
	result, err := c.service.Users.UpdateOne(
		ctx,
		bson.M{"_id": userID, "totpEnabled": true, "totpRecoveryHashes": hash},
		bson.M{"$pull": bson.M{"totpRecoveryHashes": hash}},
	)
	return err == nil && result.MatchedCount == 1
}

// issueSession creates the authenticated session and secure cookie.
func (c *Controller) issueSession(w http.ResponseWriter, r *http.Request, user models.User) {
	token := randomToken(32)
	session := models.Session{ID: token, UserID: user.ID, ExpiresAt: time.Now().Add(c.app.cfg.SessionTTL)}
	if _, err := c.service.Sessions.InsertOne(r.Context(), session); err != nil {
		writeJSON(w, 500, map[string]string{"error": "could not create session"})
		return
	}
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{Name: "session", Value: token, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: int(c.app.cfg.SessionTTL.Seconds())})
	user.Permissions = c.service.PermissionsForUser(r.Context(), user)
	writeJSON(w, 200, map[string]any{"user": user, "token": token})
}

// logout expires the current session cookie and removes its server-side session.
func (c *Controller) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, e := r.Cookie("session"); e == nil {
		_, _ = c.service.Sessions.DeleteOne(r.Context(), bson.M{"_id": cookie.Value})
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", HttpOnly: true, Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// me returns the current user's profile and effective permissions.
func (c *Controller) me(w http.ResponseWriter, r *http.Request) {
	c.app.auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, r.Context().Value(userKey)) })).ServeHTTP(w, r)
}

// accountSecurity reads or updates the current user's profile and MFA settings.
func (c *Controller) accountSecurity(w http.ResponseWriter, r *http.Request) {
	user, _ := r.Context().Value(userKey).(models.User)
	switch r.URL.Path {
	case "/api/auth/profile":
		if r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		var in struct {
			CurrentPassword string `json:"currentPassword"`
			Username        string `json:"username"`
			Email           string `json:"email"`
		}
		if bodyJSON(r, &in) != nil || !verifyPassword(in.CurrentPassword, user.PasswordHash) {
			writeJSON(w, 401, map[string]string{"error": "password verification failed"})
			return
		}
		username, email := strings.TrimSpace(in.Username), strings.TrimSpace(in.Email)
		if len(username) < 3 {
			writeJSON(w, 400, map[string]string{"error": "username must be at least 3 characters"})
			return
		}
		address, err := mail.ParseAddress(email)
		if err != nil || address.Address != email {
			writeJSON(w, 400, map[string]string{"error": "enter a valid email address"})
			return
		}
		_, err = c.service.Users.UpdateOne(r.Context(), bson.M{"_id": user.ID}, bson.M{"$set": bson.M{"username": username, "email": email}})
		if c.service.IsDuplicateKey(err) {
			writeJSON(w, 409, map[string]string{"error": "that username or email is already in use"})
			return
		}
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not update profile"})
			return
		}
		user.Username, user.Email = username, email
		writeJSON(w, 200, user)
	case "/api/auth/password":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var in struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		if bodyJSON(r, &in) != nil || len(in.NewPassword) < 8 || !verifyPassword(in.CurrentPassword, user.PasswordHash) {
			writeJSON(w, 400, map[string]string{"error": "current password is incorrect or new password is shorter than 8 characters"})
			return
		}
		hash, err := hashPassword(in.NewPassword)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "password hashing failed"})
			return
		}
		_, err = c.service.Users.UpdateOne(r.Context(), bson.M{"_id": user.ID}, bson.M{"$set": bson.M{"passwordHash": hash}})
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not update password"})
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case "/api/auth/totp/enroll":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var in struct {
			Password string `json:"password"`
		}
		if bodyJSON(r, &in) != nil || !verifyPassword(in.Password, user.PasswordHash) {
			writeJSON(w, 401, map[string]string{"error": "password verification failed"})
			return
		}
		secret, err := newTOTPSecret()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not generate authenticator secret"})
			return
		}
		_, err = c.service.Users.UpdateOne(r.Context(), bson.M{"_id": user.ID}, bson.M{"$set": bson.M{"totpSetupSecret": secret, "totpSetupExpiresAt": time.Now().Add(10 * time.Minute)}})
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not save authenticator setup"})
			return
		}
		writeJSON(w, 200, map[string]string{"secret": secret, "otpauthUrl": totpURL(user.Email, secret)})
	case "/api/auth/totp/verify":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var in struct {
			Code string `json:"code"`
		}
		if bodyJSON(r, &in) != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid request"})
			return
		}
		var current models.User
		if c.service.Users.FindOne(r.Context(), bson.M{"_id": user.ID, "totpSetupExpiresAt": bson.M{"$gt": time.Now()}}).Decode(&current) != nil || current.TOTPSetupSecret == "" || !verifyTOTP(current.TOTPSetupSecret, in.Code, time.Now()) {
			writeJSON(w, 401, map[string]string{"error": "invalid or expired authenticator code"})
			return
		}
		recoveryCodes, recoveryHashes, err := generateTOTPRecoveryCodes()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not generate authenticator recovery codes"})
			return
		}
		_, err = c.service.Users.UpdateOne(r.Context(), bson.M{"_id": user.ID}, bson.M{"$set": bson.M{"totpSecret": current.TOTPSetupSecret, "totpEnabled": true, "totpRecoveryHashes": recoveryHashes}, "$unset": bson.M{"totpSetupSecret": "", "totpSetupExpiresAt": ""}})
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not enable authenticator"})
			return
		}
		writeJSON(w, 200, map[string]any{"totpEnabled": true, "recoveryCodes": recoveryCodes})
	case "/api/auth/totp/remove":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var in struct {
			Code string `json:"code"`
		}
		if bodyJSON(r, &in) != nil {
			writeJSON(w, 400, map[string]string{"error": "enter an authenticator or recovery code"})
			return
		}
		validCode := user.TOTPEnabled && verifyTOTP(user.TOTPSecret, in.Code, time.Now())
		if !validCode && user.TOTPEnabled {
			validCode = c.consumeTOTPRecoveryCode(r.Context(), user.ID, in.Code)
		}
		if !validCode {
			writeJSON(w, 401, map[string]string{"error": "invalid authenticator or recovery code"})
			return
		}
		_, err := c.service.Users.UpdateOne(r.Context(), bson.M{"_id": user.ID, "totpEnabled": true}, bson.M{"$set": bson.M{"totpEnabled": false}, "$unset": bson.M{"totpSecret": "", "totpRecoveryHashes": "", "totpSetupSecret": "", "totpSetupExpiresAt": ""}})
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not remove authenticator"})
			return
		}
		writeJSON(w, 200, map[string]bool{"totpEnabled": false})
	case "/api/auth/totp/recovery-codes":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var in struct {
			Code string `json:"code"`
		}
		if bodyJSON(r, &in) != nil || !user.TOTPEnabled || !verifyTOTP(user.TOTPSecret, in.Code, time.Now()) {
			writeJSON(w, 401, map[string]string{"error": "invalid authenticator code"})
			return
		}
		recoveryCodes, recoveryHashes, err := generateTOTPRecoveryCodes()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not generate recovery codes"})
			return
		}
		_, err = c.service.Users.UpdateOne(r.Context(), bson.M{"_id": user.ID, "totpEnabled": true}, bson.M{"$set": bson.M{"totpRecoveryHashes": recoveryHashes}})
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "could not save recovery codes"})
			return
		}
		writeJSON(w, 200, map[string]any{"recoveryCodes": recoveryCodes})
	case "/api/auth/totp":
		if r.Method == http.MethodGet {
			writeJSON(w, 200, map[string]bool{"totpEnabled": user.TOTPEnabled})
			return
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

// transportSettings reads or updates the panel's public HTTP and HTTPS settings.
func (c *Controller) transportSettings(w http.ResponseWriter, r *http.Request) {
	actor, _ := r.Context().Value(userKey).(models.User)
	if actor.Role != "root" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only the root user can view or change HTTP/HTTPS settings"})
		return
	}
	settings := c.service.Settings
	if r.Method == http.MethodGet {
		var current struct {
			HTTPSRequired    bool   `bson:"httpsRequired" json:"httpsRequired"`
			HTTPSDomain      string `bson:"httpsDomain" json:"httpsDomain"`
			CertificateEmail string `bson:"certificateEmail" json:"certificateEmail"`
		}
		_ = settings.FindOne(r.Context(), bson.M{"_id": "transport"}).Decode(&current)
		writeJSON(w, http.StatusOK, current)
		return
	}
	if r.Method != http.MethodPut {
		http.NotFound(w, r)
		return
	}
	var in struct {
		HTTPSRequired    bool   `json:"httpsRequired"`
		HTTPSDomain      string `json:"httpsDomain"`
		CertificateEmail string `json:"certificateEmail"`
	}
	if bodyJSON(r, &in) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	in.HTTPSDomain = strings.ToLower(strings.TrimSpace(in.HTTPSDomain))
	in.CertificateEmail = strings.TrimSpace(in.CertificateEmail)
	if in.HTTPSDomain != "" && !validPublicHostname(in.HTTPSDomain) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enter a valid public DNS hostname such as panel.example.com"})
		return
	}
	if in.CertificateEmail != "" {
		if parsed, err := mail.ParseAddress(in.CertificateEmail); err != nil || parsed.Address != in.CertificateEmail {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enter a valid certificate contact email"})
			return
		}
	}
	if in.HTTPSRequired {
		if in.HTTPSDomain == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "save a public hostname before requiring HTTPS"})
			return
		}
		secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
		if !secure {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "install the TLS proxy and open the panel over HTTPS before requiring HTTPS"})
			return
		}
		host := r.Host
		if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
			host = strings.TrimSpace(strings.Split(forwarded, ",")[0])
		}
		if parsedHost, _, err := net.SplitHostPort(host); err == nil {
			host = parsedHost
		}
		if !strings.EqualFold(strings.TrimSuffix(host, "."), in.HTTPSDomain) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "open the panel using the configured HTTPS hostname before requiring HTTPS"})
			return
		}
		if addresses, err := net.LookupHost(in.HTTPSDomain); err != nil || len(addresses) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "the configured hostname must resolve before HTTPS can be required"})
			return
		}
	}
	value := bson.M{"httpsRequired": in.HTTPSRequired, "httpsDomain": in.HTTPSDomain, "certificateEmail": in.CertificateEmail}
	_, err := settings.UpdateOne(r.Context(), bson.M{"_id": "transport"}, bson.M{"$set": value}, true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save transport settings"})
		return
	}
	c.app.httpsRequired.Store(in.HTTPSRequired)
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		if in.HTTPSRequired {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		} else {
			w.Header().Set("Strict-Transport-Security", "max-age=0")
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"httpsRequired": in.HTTPSRequired, "httpsDomain": in.HTTPSDomain, "certificateEmail": in.CertificateEmail})
}

// validPublicHostname accepts DNS hostnames suitable for public transport settings.
func validPublicHostname(host string) bool {
	if len(host) > 253 || net.ParseIP(host) != nil {
		return false
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' {
				return false
			}
		}
	}
	return true
}
