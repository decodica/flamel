package model

import (
	"fmt"
	"google.golang.org/appengine/aetest"
	"google.golang.org/appengine/memcache"
	"testing"
)

type Entity struct {
	Model
	Name       string
	Num        int
	Child      Child
	EmptyChild EmptyChild `model:"zero"`
}

type Child struct {
	Model
	Name       string
	Grandchild Grandchild
}

type Grandchild struct {
	Model
	GrandchildNum int
}

type EmptyChild struct {
	Model
	Emptiness int
}

const total = 100
const find = 10

func TestIndexing(t *testing.T) {

	ctx, done, err := aetest.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	// test correct indexing
	entity := Entity{}
	index(&entity)
	if !entity.EmptyChild.skipIfZero {
		t.Fatal("empty child is not skipIfZero")
	}

	entity.Name = "entity"
	entity.Child.Name = "child"
	err = Create(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = Read(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	if entity.EmptyChild.Key != nil {
		t.Fatal("empty child has non-nil key")
	}
}

func TestUpdate(t *testing.T) {
	ctx, done, err := aetest.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer done()

	// test correct indexing
	entity := Entity{}
	entity.Child.Name = "child"
	entity.Child.Grandchild.GrandchildNum = 10

	err = Create(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = Read(ctx, &entity)
	if err != nil {
		t.Fatal(err)
	}
	entity.Child.Grandchild.GrandchildNum = 100
	entity.Child.Name = ""

	err = Update(ctx, &entity)
	if err != nil {
		t.Fatal(err.Error())
	}

	if entity.EmptyChild.Key != nil {
		t.Fatal("empty child has non-nil key after update")
	}

	if entity.Child.Grandchild.GrandchildNum != 100 {
		t.Fatalf("grand child has not been updated. Num is %d", entity.Child.Grandchild.GrandchildNum)
	}

	if entity.Child.Name != "" {
		t.Fatalf("child has not been updated. Name is %s", entity.Child.Name)
	}
}

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

	if len(dst) != total-find-1 {
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
