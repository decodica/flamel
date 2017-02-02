package blueprint

import "google.golang.org/appengine/datastore"

//slice of dataMaps. It is sortable by each dataMap key
type dataMaps []*dataMap;

func (data dataMaps) Len() int {
	return len(data);
}

func (data dataMaps) Swap(i int, j int) {
	data[i] = data[j];
	data[j] = data[i];
}

func (data dataMaps) Less(i int, j int) bool {
	return data[i].key.Encode() < data[j].key.Encode();
}

func (data dataMaps) Keys() ([]*datastore.Key) {
	keys := make([]*datastore.Key, data.Len(), data.Len());
	for i, v := range data {
		keys[i] = v.key;
	}

	return keys;
}

//Organizes dataMaps into a map having the storage key as the key and the associated dataMap as a value.
//returns the newly created map (allocated) and a slice containing the indexes of possible nil keys
func (data dataMaps) KeyMap() (map[*datastore.Key]*dataMap, []int) {
	keyMap := make(map[*datastore.Key]*dataMap, data.Len());
	nilIdx := make([]int, 0);
	for i, v := range data {
		k := v.key;
		if k == nil {
			nilIdx = append(nilIdx, i);
		}
		keyMap[k] = data[i];
	}
	return keyMap, nilIdx;
}
