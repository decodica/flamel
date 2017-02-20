package user

import (
	"distudio.com/mage/model"
	"golang.org/x/net/context"
	"fmt"
)

type UserType int64;

//Default user for mage applications.
type User struct {
	model.Model
	Username   string
	Password   string
	Token      string
	UserType   UserType
}

func NewUser(ctx context.Context) *User {
	return &User{};
}

//populate the user from a given key
func (user *User) Authenticate(ctx context.Context, token string) error {
	//recupero key da token in memcache
	query := model.NewQuery(user);
	query.WithField("Token =", token);

	res := make([]*User, 0);

	err := query.GetAll(ctx, &res);

	if err != nil {
		user.Logout();
		return err;
	}

	if len(res) > 1 {
		user.Logout();
		return fmt.Errorf("Found %d users for token %s. Invalid access", len(res), token);
	}

	if len(res) < 1 {
		user.Logout();
		return fmt.Errorf("Found no users for token %s. Invalid access", token);
	}

	*user = *(res[0]);
	user.Token = token;
	return nil;
}


func (user User) IsAuthenticated() bool {
	return user.Token != "";
}

func (user *User) Logout() {
	user.Token = "";
}