package domain

// PendingLogin is a sign-in that was started and waits for authentik to redirect back.
type PendingLogin struct {
	Nonce    string
	Verifier string // PKCE code verifier
}

// Session is a signed-in browser. It keeps the identity from the ID token, so requests
// made with a session need no call to authentik.
type Session struct {
	Identity Identity
	IDToken  string // sent back to authentik as a hint when signing out
}
