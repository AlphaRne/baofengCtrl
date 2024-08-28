package main

import (
	"fmt"
	"go.bug.st/serial"
	"flag"
	"strings"
	"log"
	"os"
	"errors"
	"time"
	"bytes"
)

type BfIo struct{
	Port serial.Port
	BlockSize uint8
}

var devName string
var baudRate int


func HexString(data []byte) string {
	var buf bytes.Buffer
	for i := 0; i < len(data)-1; i++ {
		buf.WriteString(fmt.Sprintf("%02X ", data[i]))
	}
	buf.WriteString(fmt.Sprintf("%02X", data[len(data)-1]))
	return buf.String()
}

func HexStringS(data []byte, sep string) string {
	var buf bytes.Buffer
	for i := 0; i < len(data)-1; i++ {
		buf.WriteString(fmt.Sprintf("%02X%s", data[i], sep))
	}
	buf.WriteString(fmt.Sprintf("%02X", data[len(data)-1]))
	return buf.String()
}



func initCmdline() {
	flag.StringVar(&devName, "dev", "", "serial port device")
	flag.IntVar(&baudRate, "b", 115200, "baud rate")
	flag.Parse()
}


func (b *BfIo) sendReceive(tx []uint8, rspCount int) ([]uint8, error){
	b.Port.Write(tx)
	res := make([]uint8, rspCount)
	ofs := 0
	for{
		n, err := b.Port.Read(res[ofs:])
		if nil != err{
			return nil, err
		}
		ofs += n
		if ofs == rspCount{
			return res, nil
		}else if 0 == n{
			return res[:ofs], errors.New(fmt.Sprintf("short read [%x %x] ",rspCount, ofs))
		}
	}	
	return res, nil
}

func (b *BfIo) initialComm() error{
	if res, err := b.sendReceive([]uint8("PROGRAMBFNORMALU"), 1); nil != err{
		return err
	}else{
		fmt.Printf("res:%v\n", HexString(res))
	}
	/*
		send some data from 0xF230
	*/
	if res, err := b.sendReceive([]uint8{'F'}, 16);nil != err{
		return err
	}else{
		fmt.Printf("res:%v\n", HexString(res))
	}

	// return some device REV[8] ,'+','L', 5 bytes from flash 0xF251
	if res, err := b.sendReceive([]uint8{'M'}, 15);nil != err{
		return err
	}else{
		fmt.Printf("res:%v\n", string(res))
	}

	/* 
		key Selection:
		Cmd: "SEND" + DATA...
		var idx int
		if data[0] >= 0x20{
			a := data[0] - 0x20
			if a <= 4{
				idx = 2 * a + 2
			}else {
				return -1
			}
		}else{
			a := data[0] - 0x10
			if a <= 4{
				idx = 2 * a + 1
			}else{
				return -1
			}
		}
		keyIndex := data[idx]
		if keyIndex > 0x13{
			return -1
		}
	*/
	if res, err := b.sendReceive([]uint8{'S','E','N','D',0x21,0x05,0x0D,0x01,0x01,0x01,0x04,0x11,0x08,0x05,0x0D,0x0D,0x01,0x11,0x0F,0x09,0x12,0x09,0x10,0x04,0x00},1);nil != err{
		return err
	}else{
		fmt.Printf("res:%v\n", HexString(res))
	}

	return nil
}

func (b *BfIo) ReadBlock(addr uint16) ([]uint8, error){
	cmd := []uint8{'R', uint8(addr >> 8), uint8(addr), b.BlockSize}
	if res, err := b.sendReceive(cmd, 4 + int(b.BlockSize)); nil != err{
		return res, err
	}else{
		if 0x52 != res[0] || uint8(addr>>8) != res[1] || uint8(addr) != res[2]{
			return res[4:], errors.New("wrong read response")
		}
	//	fmt.Printf("read[%04X]:%v\n", HexString(res))
		return res[4:], nil
	}
}


func (b *BfIo) WriteBlock(addr uint16, data []uint8) error{
	cmd := make([]uint8, b.BlockSize + 4)
	cmd[0] = 'W'
	cmd[1] = uint8(addr >> 8)
	cmd[2] = uint8(addr)
	cmd[3] = b.BlockSize
	copy(cmd[4:], data)
	if res, err := b.sendReceive(cmd, 1); nil != err{
		return err
	}else{
		if 0x06 != res[0]{
			return errors.New("wrong write response")
		}
	}
	return nil
}


func  (b *BfIo) ReadMemory(keyIndex int, addr uint16, count int) ([]uint8, error){
	res := make([]uint8, count)
	for i := 0; i < count; i += int(b.BlockSize){
		buf, err := b.ReadBlock(addr)
		if nil != err{
			return res, err
		}
		if keyIndex >= 0{
			buf2 := b.Crypt(buf, keyIndex)
			copy(res[i:], buf2)
		}else{
			copy(res[i:], buf)
		}
		addr += uint16(b.BlockSize)
	}
	return res, nil
}


func (b *BfIo) WriteMemory(keyIndex int, addr uint16, data []uint8) error{
	for i := 0; i < len(data); i += int(b.BlockSize){
		var d []uint8
		if keyIndex >= 0{
			d = b.Crypt(data[i:i+int(b.BlockSize)], keyIndex)
		}else{
			d = data[i:i+int(b.BlockSize)]
		}
		err := b.WriteBlock(addr, d)
		if nil != err{
			return err
		}
		addr += uint16(b.BlockSize)
	}
	return nil
}


func (b  *BfIo) Crypt(data []uint8, index int) []uint8{
	res := make([]uint8, len(data))
	keys := []uint8("BHT "+"CO 7"+"A ES" +" EIY" + "M PQ" + "XN Y" + "RVB " + " HQP" + "W RC" + "MS N" + " SAT" + "K DH" + "ZO R" + "C SL" + "6RB " + " JCG" + "PN V" + "J PK" + "EK L" + "I LZ")
	key := keys[4 * index:4*index + 4]
	j := 0
	for i := 0; i < len(data);i++{
		if 0x20 != key[j] && 0 != data[i] && 0xFF != data[i] && key[j] != data[i] && (key[j] ^ data[i]) != 0xff{
			res[i] = data[i] ^ key[j]
		}else{
			res[i] = data[i]
		}
		j = (j + 1) & 0x3
	}
	return res
}


func main(){
	initCmdline()
	if "" == devName{
		entries, err := os.ReadDir("/dev")
   		if err != nil {
        	log.Fatal(err)
    	}
    	for _, e := range entries {
    		if strings.HasPrefix(e.Name(), "cu.usbmodem"){
    			devName = "/dev/" + e.Name()
    			fmt.Printf("using %s\n",e.Name())
    			break
    		}
           
    	}
	}
	mode := &serial.Mode{
		BaudRate: baudRate,
		Parity: serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}	

	p, err := serial.Open(devName, mode)
	if nil != err {
		fmt.Printf("error %v\n", err)
		return
	}
	p.ResetInputBuffer()
	p.SetReadTimeout(time.Second)
	bf := &BfIo{Port: p, BlockSize:0x40}

	if err := bf.initialComm();nil != err{
		fmt.Printf("error %v\n", err)
		return
	}

	p.ResetInputBuffer()
	// 0xF000, erase executed when addr is sector start (0xF000)



	bd, err := bf.ReadMemory(-1, 0xF000, 0x1000)
	if nil != err{
		fmt.Printf("error %v %v\n", err, HexString(bd))
		return
	}else{
		fmt.Printf("block data %x\n", bd[0x255])
	}
	bd[0x255] = '0'
//	err = bf.WriteMemory(-1, 0xF000, bd)
//	if nil != err{
//		fmt.Printf("error %v\n", err)
//		return
//	}

	d, err := bf.ReadBlock(0xF240)
	if nil != err{
		fmt.Printf("error %v %v\n", err, HexString(d))
		return
	}
//	d2 := bf.Crypt(d, 2)
	fmt.Printf("Block Data %x\n", d[0x15])


	fmt.Printf("Hello\n")
}
