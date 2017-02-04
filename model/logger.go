package model

import "log"


func gPrintf(formatString string, v ...interface{}) {
	log.Printf(formatString, v);
}

func gPrint(v ...interface{}) {
	log.Print(v);
}


