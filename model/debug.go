package model

/*import (
	"strconv"
	"fmt"
	"strings"
	"log"
)

func (data dataMap) Printf(formatString string, v ...interface{}) {
	if data.Debug {
		name := data.entityName;
		log.Printf(name + "]:" + formatString, v);
	}
}

func (data dataMap) Print(s string) {
	if data.Debug {
		log.Print(s);
	}
}

func (model *dataMap) Dump() {
	s := "\n";
	s += "========== DUMPING MODEL FOR PROTOTYPE " + model.entityName + " =========\n";
	if model.key != nil {
		s += "-- KEY: " + model.key.Encode() + "\n";
		s += "-- ID " + strconv.Itoa(int(model.key.IntID())) + "\n";
	} else {
		s += "-- KEY IS NIL \n";
	}

	s += fmt.Sprintf("-- VALUE %+v", model.Prototype());
	s += model.dumpReferences(0);
	d := model.Debug;
	model.Debug = true;
	model.Print(s);
	model.Debug = d;
}

func (model *dataMap) dumpReferences(nest int) string {

	h := strings.Repeat("--", nest + 1);
	s  := "\n" + h + "-- LEVEL " + strconv.Itoa(nest) + " REFS - MODEL " + model.entityName + ": \n";
	hasRef := false;
	for k, ref := range model.references {
		hasRef = true;
		sindex := strconv.Itoa(k);
		s += h + "-- REF " + ref.entityName + " AT INDEX " + sindex + "\n";
		if ref.key != nil {
			s += h + "-- REF HAS KEY " + ref.key.Encode() + "\n";
			s += h + "-- REF HAS ID " + strconv.Itoa(int(ref.key.IntID())) + "\n";
		} else {
			s += h + "-- REF HAS NIL KEY \n";
		}

		s += fmt.Sprintf(h + "-- REF HAS VALUE %+v", ref.Prototype());

		for _, rr := range ref.references {
			s +=rr.dumpReferences(nest + 1);
		}
	}
	if !hasRef {
		s += h + "-- NO REFS FOUND FOR MODEL " + model.entityName + "\n";
	}
	return s;
}

func (model *Model) DeepDebug(debug bool) {
	for _, ref := range model.references {
		ref.DeepDebug(debug);
	}
	model.Debug = debug;
}*/
