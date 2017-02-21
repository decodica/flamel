package mage

import (
	"golang.org/x/net/context"
)

type CookieAuthenticator struct {}

func (authenticator CookieAuthenticator) Authenticate(ctx context.Context, user Authenticable) context.Context {
	inputs := InputsFromContext(ctx);
	//we get the cookie with the selected/default key
	key := MageInstance().Config.TokenAuthenticationKey;

	if tkn, ok := inputs[key]; ok {
		err := user.Authenticate(ctx, tkn.Value())
		if err != nil {
			user = nil;
		}
		//put the user into the context together with the other params
		return context.WithValue(ctx, REQUEST_USER, user);
	}

	return ctx;
}