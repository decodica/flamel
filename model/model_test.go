package model

import (
	"fmt"
	"google.golang.org/appengine/aetest"
	"google.golang.org/appengine/memcache"
	"testing"
)


type Entity struct {
	Model
	Name string
	Num int
	Child Child
	EmptyChild EmptyChild
}

type Child struct {
	Model
	Name string
	Grandchild Grandchild
}

type Grandchild struct {
	Model
	GrandchildNum int
}

type EmptyChild struct {
	Emptiness int
}

const total = 100
const find = 10

func TestModel(t *testing.T) {

	ctx, done, err := aetest.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	for i := 0; i < total; i++ {
		entity := Entity{}
		entity.Name = fmt.Sprintf("%d", i)
		entity.Num = i
		entity.Child.Name = fmt.Sprintf("child-%s", entity.Name)
		entity.Child.Grandchild.GrandchildNum = i
		err := Create(ctx, &entity)
		if err != nil {
			t.Fatal(err)
		}
	}

	err = memcache.Flush(ctx)
	if err != nil {
		t.Fatal(err)
	}

	dst := make([]*Entity, 0)

	q := NewQuery(&Entity{})
	q = q.WithField("Num >", find)
	err = q.GetMulti(ctx, &dst)
	if err != nil {
		t.Fatal(err)
	}

	if len(dst) != total - find - 1 {
		t.Fatalf("invalid number of data returned. Count is %d", len(dst))
	}

	for k := find + 1; k < total; k++ {
		idx := k - find - 1
		entity := dst[idx]
		if entity.Num != k {
			t.Fatalf("invalid error. entity at index %d has value %d.", idx, entity.Num)
		}
	}
}
