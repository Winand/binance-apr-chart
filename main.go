package main

import (
	"database/sql"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"

	_ "github.com/mattn/go-sqlite3" // https://stackoverflow.com/a/21225073
)

type timeSlice []time.Time

func (s timeSlice) Less(i, j int) bool { return s[i].Before(s[j]) }
func (s timeSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s timeSlice) Len() int           { return len(s) }

type CsvData struct {
	mu       sync.RWMutex
	records  [][]string
	dates    []string
	dat      map[string][]float64
	datNew   map[string]map[time.Time]float32
	datesNew timeSlice
}

var file *os.File
var reader *csv.Reader
var data *CsvData
var db_name = "binance_apr.sqlite"

func updateDataFromCSV() {
	for {
		// Читаем данные из файла
		file.Seek(0, 0) // переводим указатель на начало файла
		records, err := reader.ReadAll()
		if err != nil {
			panic(err)
		}

		// Обновляем объект CsvData новыми данными
		data.mu.Lock()
		data.records = records
		data.dates = []string{}
		data.dat = map[string][]float64{} //{"hello": "world", …}
		for _, line := range data.records[1:] {
			// fmt.Println(line[0], line[1], line[2])
			v, _ := strconv.ParseFloat(line[2], 64)
			data.dat[line[1]] = append(data.dat[line[1]], v)
			if len(data.dates) == 0 || data.dates[len(data.dates)-1] != line[0] {
				data.dates = append(data.dates, line[0])
			}
		}
		// https://stackoverflow.com/a/62079701
		// bs, _ := json.Marshal(dat)
		// fmt.Println(string(bs))
		data.mu.Unlock()

		// Обновляем содержимое файла раз в 5 минут
		time.Sleep(5 * time.Minute)
	}
}

func updateDataFromDB() {
	db, err := sql.Open("sqlite3", db_name)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	rows, err := db.Query("select time, asset, apy from apr")
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	// Обновляем объект CsvData новыми данными
	data.mu.Lock()
	data.dates = []string{}
	data.datNew = map[string]map[time.Time]float32{}
	dateSet := map[time.Time]struct{}{} // set: https://golang-blog.blogspot.com/2020/04/set-implementation-in-golang.html
	for rows.Next() {
		var dt time.Time
		var asset string
		var apy float32
		if err := rows.Scan(&dt, &asset, &apy); err != nil {
			log.Fatal(err)
		}
		// fmt.Printf("time=%s, asset=%s, apy=%f\n", dt.String(), asset, apy)
		if data.datNew[asset] == nil {
			data.datNew[asset] = map[time.Time]float32{}
		}
		data.datNew[asset][dt] = apy
		dateSet[dt] = struct{}{}
	}
	data.mu.Unlock()

	// map sort https://www.geeksforgeeks.org/how-to-sort-golang-map-by-keys-or-values/
	// time sort https://www.socketloop.com/tutorials/golang-time-slice-or-date-sort-and-reverse-sort-example
	// zero length, capacity = len(dates)
	data.datesNew = make(timeSlice, 0, len(data.datesNew))
	for k := range dateSet {
		data.datesNew = append(data.datesNew, k)
	}
	sort.Sort(data.datesNew)
}

func updateDataFromDBLoop() {
	for {
		updateDataFromDB()
		// Обновляем содержимое файла раз в 5 минут
		time.Sleep(5 * time.Minute)
	}
}

func generateLineItems(vals []float64) []opts.LineData {
	items := make([]opts.LineData, 0)
	for i := 0; i < len(vals); i++ {
		items = append(items, opts.LineData{Value: vals[i]})
	}
	return items
}

func generateLineItemsNew(dates timeSlice, vals map[time.Time]float32) []opts.LineData {
	items := make([]opts.LineData, len(dates))
	for i, t := range dates {
		if v, found := vals[t]; found {
			items[i] = opts.LineData{Value: v}
		}
	}
	return items
}

func httpserver(w http.ResponseWriter, _ *http.Request) {
	data.mu.RLock()
	defer data.mu.RUnlock()

	// create a new line instance
	line := charts.NewLine()
	// set some global options like Title/Legend/ToolTip or anything else
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{Theme: types.ThemeWesteros}),
		charts.WithTitleOpts(opts.Title{
			Title:    "Binance Earn APR History",
			Subtitle: "Line chart rendered by the http server this time",
		}),
		charts.WithLegendOpts(opts.Legend{Show: true}),
	)

	// Put data into instance
	line.SetXAxis(data.datesNew).SetSeriesOptions(
		charts.WithLineChartOpts(opts.LineChart{Smooth: true}),
		// charts.WithLabelOpts(opts.Label{Show: true}),
	)
	for asset, vals := range data.datNew {
		line.AddSeries(asset, generateLineItemsNew(data.datesNew, vals))
	}
	line.Render(w)
}

func convert_csv_to_sqlite() {
	file, err := os.Open("binance_apr.csv")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	reader = csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		panic(err)
	}

	db, err := sql.Open("sqlite3", db_name)
	if err != nil {
		log.Fatal(err) // https://stackoverflow.com/q/35996966
	}
	defer db.Close()

	// without rowid https://dba.stackexchange.com/a/265930
	_, err = db.Exec(`
		create table 'apr' (
			time datetime, asset string, apy float, bonus float,
			primary key (time, asset)
		) without rowid
	`)
	if err != nil {
		fmt.Println(err)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		fmt.Println(err)
		return
	}
	// Prepare statement
	stmt, err := tx.Prepare(
		`insert into apr (time, asset, apy, bonus) VALUES (unixepoch(?), ?, ?, ?);`,
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer stmt.Close()

	cnt := 0
	for _, line := range records[1:] {
		_, err = stmt.Exec(line[0], line[1], line[2], line[3])
		if err != nil {
			fmt.Println(err)
			return
		}
		cnt += 1
		if cnt%100 == 0 {
			println(cnt)
		}
	}
	tx.Commit()

	fmt.Println("Data inserted successfully")
}

func main() {
	convFlag := flag.Bool("convert", false, "Convert CSV to SQLite")
	flag.Parse()
	if *convFlag {
		println("Conversion...")
		convert_csv_to_sqlite()
		os.Exit(0)
	}
	file, err := os.Open("binance_apr.csv")
	if err != nil {
		panic(err)
	}
	defer file.Close()
	reader = csv.NewReader(file)
	data = &CsvData{}

	go updateDataFromDBLoop()
	// go updateDataFromCSV()

	http.HandleFunc("/", httpserver)
	// Open port in firewall https://linuxconfig.org/how-to-allow-port-through-firewall-on-almalinux
	// firewall-cmd --zone=public --add-port 8081/tcp --permanent
	// firewall-cmd --reload
	// firewall-cmd --zone=public --list-ports
	http.ListenAndServe(":8081", nil)
}
