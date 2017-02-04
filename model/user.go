package model

/*import (
	"google.golang.org/appengine/memcache"
	"google.golang.org/appengine/datastore"
	"log"
	"golang.org/x/net/context"
)*/

type UserType int64;

type User struct {
	*Model;
}

type UserProto struct {
	Prototype `datastore:"-"`
	Authenticable `datastore:"-"`
	Username   string
	Password   string
	Token      string
	UserType   UserType
}

type Authenticable interface {
	Authenticate(token string);
}

/*func NewUser(c context.Context) Authenticable {
	u := &UserProto{};
	m, _ := NewModel(c, u);
	return &User{Model:m};
}

//populate the user from a given key
func (user *User) Authenticate(token string) {

	u := user.Prototype().(*UserProto);
	//recupero key da token in memcache
	item, e := memcache.Get(user.context, u.Token);

	if e != nil {
		return;
	}

	userKey := string(item.Value);
	user.key, _ = datastore.DecodeKey(userKey);

	e = user.read();

	if nil != e {
		panic(e);
	}

	u.Token = token;
}

func (user *User) SetPassword(password string) {
	u := user.m.(*UserProto);
	u.Password = password;
}

func (user *User) Password() string {
	u := user.m.(*UserProto);
	return u.Password;
}

func (user *User) SetUsername(username string) {
	u := user.m.(*UserProto);
	u.Username = username;
}

func (user *User) Username() string {
	u := user.m.(*UserProto);
	return u.Username;
}

func (user *User) UserType() UserType {
	u := user.dataMap.m.(*UserProto);
	return u.UserType;
}

func (user User) IsAuthenticated() bool {
	return user.Id() > -1;
}

func (user *User) Login() error {
	u := user.dataMap.m.(*UserProto);
	i := &memcache.Item{};
	i.Key = u.Token;
	i.Value = []byte(user.key.Encode());
	err := memcache.Set(user.context, i);

	if err == nil {
		err = user.Model.Update();
	}


	return err;
}

func (user *User) Logout() error {
	u := user.dataMap.m.(*UserProto);
	//remove token from memcache

	//todo: why cachemiss?
	log.Print(user.Id())
	e := memcache.Delete(user.context, u.Token);
	u.Token = "";


	return e;
}*/

