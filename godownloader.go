package main

/*
	Attempting to make a multi-threaded downloader that uses
	all available network interfaces.
*/

import (
	"./net"
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type Chunk struct {
	//Will use this to define a chunk
	Start      int64
	End        int64
	Payload    []byte
	Url        string
	TimeTaken  time.Duration
	WorkerName string
}

func worker(name string, client *http.Client, in chan Chunk, out chan Chunk, fail chan Chunk) {
	log.Printf("%s: Initialized\n", name)
	idlestart := time.Now()
	for {
		chunk := <-in
		log.Printf("%s: was idle for %s\n", name, time.Since(idlestart))
		chunk.WorkerName = name //We use this later
		start := time.Now()
		log.Printf("%s: Start: %v - %v\n", name, chunk.Start, chunk.End)
		req, err := http.NewRequest("GET", chunk.Url, nil)
		if err != nil {
			log.Printf("%s: Error: %v - %v  %v", name, chunk.Start, chunk.End, err)
			//Reque
			fail <- chunk
		} else {
			req.Header.Add("Range", fmt.Sprintf("bytes=%v-%v", chunk.Start, chunk.End))
			resp, err := client.Do(req)
			if err != nil {
				log.Printf("%s: Error: %v - %v\n", name, chunk.Start, chunk.End)
				log.Println(err)
				//Reque
				fail <- chunk
				//close(in)
				//return // Close this shit
				//time.Sleep(5 * time.Second) //Don't take job for 5 seconds
			} else {
				if resp.StatusCode != 200 && resp.StatusCode != 206 {
					log.Printf("%s: Error: %v - %v\nResp not 200/206 but %v", name, chunk.Start, chunk.End, resp.StatusCode)
					//Reque
					fail <- chunk
					//close(in)
					//return // Close this shit
				} else {
					//Should actually evaluate the headers for returned range and tell it to the upcoming smarter allocator
					chunk.Payload, err = ioutil.ReadAll(resp.Body) //Read everything. prolly a bad idea
					if err != nil {
						log.Printf("%s: Error: %v - %v\n", name, chunk.Start, chunk.End)
						log.Println(err)
						//Reque
						fail <- chunk
						//close(in)
						//return // Close this shit
					} else {
						chunk.TimeTaken = time.Since(start) //TODO: Use this timetaken somehow...
						log.Printf("%s: Done: %v - %v in %s\n", name, chunk.Start, chunk.End, chunk.TimeTaken)
						idlestart = time.Now()
						out <- chunk
					}
				}
			}
		}
	}
}

func filesaver(fname string, out chan Chunk, totalsize int64, done chan bool) {
	var stored int64
	f, err := os.Create(fname)
	if err != nil {
		log.Panic(err)
	}
	defer f.Close()
	log.Printf("filesaver: Initialized\n")

	for {
		//fmt.Printf("main: %v of %v chunks remain\n", chunknum-done, chunknum)
		chunk := <-out //chunk.Payload is the obj unless something bad happened
		log.Printf("filesaver: Got: %v - %v: Length %v\n", chunk.Start, chunk.End, len(chunk.Payload))
		_, err := f.WriteAt(chunk.Payload, chunk.Start)
		if err != nil {
			log.Panic(err)
		}
		//log.Println(n)
		stored += chunk.End - chunk.Start
		log.Printf("filesaver: %v of %v saved\n", stored, totalsize)
		if stored == totalsize {
			done <- true
			return
		}
	}
}

type instance struct {
	Name             string
	IP               net.IP
	Currentchunksize int64
	Que              chan Chunk
}

func getallocation(totalsize, allocated, chunksize int64, url string, c chan Chunk) int64 {
	//TODO: Need a smarter allocator that tracks slices allocated...
	start := allocated
	end := allocated + chunksize
	if end > totalsize {
		end = totalsize
	}
	chunk := Chunk{Url: url, Start: start, End: end}
	allocated = end
	c <- chunk
	log.Printf("allocator: Allocated %v bytes\n", allocated)
	return allocated
}

func downloadr(fname string, url string, totalsize, defaultchunksize int64, instances []instance, expectedhash string) {
	//fname : destination
	//url: The source for the object
	//defaultchunksize : Starting chunk size. Eventually chunk size will be adjusted to target 5 seconds per chunk
	//instances : Available network interfaces
	//expectedhash: MD5 checksum of the file.
	var allocated int64
	//totalsize = 52428800
	out := make(chan Chunk, 10)  //main que
	fail := make(chan Chunk, 10) //FAIL que
	instancemap := make(map[string]instance)
	//Initialize the downloaders
	for _, instance := range instances {
		instance.Currentchunksize = defaultchunksize
		instance.Que = make(chan Chunk, 2)
		//Init the worker
		go worker(instance.Name, godownloader.GetClient(instance.IP), instance.Que, out, fail)
		//Allocate the first chunks
		allocated = getallocation(totalsize, allocated, defaultchunksize, url, instance.Que)
		instancemap[instance.Name] = instance
	}
	realout := make(chan Chunk, 10) //main que
	done := make(chan bool, 1)
	go filesaver(fname, realout, totalsize, done)
	var fails []Chunk
	for {
		select {
		case chunk := <-out:
			log.Printf("main: Got: %v - %v: Length %v\n", chunk.Start, chunk.End, len(chunk.Payload))
			log.Printf("main: allocated: %v of %v\n", allocated, totalsize)
			instance := instancemap[chunk.WorkerName]
			if allocated < totalsize {
				//sizedone := chunk.End - chunk.Start
				ratio := 10 / chunk.TimeTaken.Seconds()                //target 10 second per chunk
				newchunksize := ratio * float64(chunk.End-chunk.Start) // Retarget
				//fmt.Printf("%v\n",  )
				instance.Currentchunksize = int64(newchunksize)
				allocated = getallocation(totalsize, allocated, int64(newchunksize), url, instance.Que)
			}
			realout <- chunk
		case chunk := <-fail:
			//This chunk failed... do something with is...
			log.Printf("main: FAIL from %s: %v - %v\n", chunk.WorkerName, chunk.Start, chunk.End)
			fails = append(fails, chunk)
			instance := instancemap[chunk.WorkerName]
			instance.Currentchunksize = 100
			//The worker will not send anything to out... So it will likely not get any more chunks...
		case <-time.After(10 * time.Second):
			//Every 10 seconds check for a fail que it into the best perfoming worker
			if len(fails) > 0 {
				var failchunk Chunk
				failchunk, fails = fails[len(fails)-1], fails[:len(fails)-1]
				var bestinstance instance
				var bestsize int64
				for _, i := range instancemap {
					if i.Currentchunksize > bestsize {
						bestinstance = i
						bestsize = i.Currentchunksize
					}
				}
				bestinstance.Que <- failchunk
			}
		case <-done:
			log.Printf("main: Got done from filesaver\n")
			if expectedhash != "" {
				//Match md5 only if we have something to match...
				h := md5.New()
				f, err := os.Open(fname)
				if err != nil {
					log.Panic(err)
				}
				defer f.Close()
				io.Copy(h, f) //copy file contents to hasher
				log.Printf("Hash : %x\n", h.Sum(nil))
				if expectedhash != fmt.Sprintf("%x", h.Sum(nil)) {
					log.Panic("HASH DIDNT MATCH!!!!")
				}

			}
			return
		}
	}
}

func main() {
	url := flag.String("url", "http://aarontestsg.s3.amazonaws.com/50MB.zip", "url to fetch")
	totalsize := flag.Int64("totalsize", 52428800, "Total size in bytes of the object (will be autodetected eventually)")
	basechunksize := flag.Int64("basechunksize", 100000, "Size in bytes of first chunk per interface")
	md5 := flag.String("md5sum", "", "MD5sum leave blank to skip intigrity check")
	fname := flag.String("fname", "", "Where to store filename? If blank chooses one based on filename in url")
	flag.Parse()
	if *fname == "" {
		things := strings.Split(*url, "/")
		*fname = things[len(things)-1]
	}
	log.Printf("Using %s as download target", *fname)
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Fatal(err)
	}
	instances := []instance{instance{Name: "default", IP: nil}} //Add an instance with no source addr mischief.
	for _, iface := range ifaces {
		isup := iface.Flags&(1<<uint(0)) != 0 //Is this interface up?
		islo := iface.Flags&(1<<uint(2)) != 0 //Is this interface loopback?
		log.Printf("%s\tIndex: %v\tMTU: %v\t%s\n", iface.Name, iface.Index, iface.MTU, iface.Flags)
		if isup && !islo {
			addrs, err := iface.Addrs()
			if err != nil {
				log.Fatal(err)
			}
			for _, addr := range addrs {
				//fmt.Printf("%s\n" , addr) //Somehow gives a subnet
				//fmt.Printf("%s\n" , strings.Split(addr.String(), "/")[0]) //Is this safe/accurate always?
				ip := net.ParseIP(strings.Split(addr.String(), "/")[0])
				if ip.DefaultMask() != nil {
					//Only IPv4 addresses have default masks; DefaultMask returns nil if ip is not a valid IPv4 address
					instances = append(instances, instance{Name: iface.Name, IP: ip})
				}
			}
		}
	}
	log.Printf("Instances: %v\n", instances)
	if len(instances) == 0 {
		log.Fatal("No interface detect! u has interwebs?")
	}
	downloadr(*fname, *url, *totalsize, *basechunksize, instances, *md5)
}
