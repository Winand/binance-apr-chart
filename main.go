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

type AssetsData struct {
	mu    sync.RWMutex
	dat   map[string]map[time.Time]float32
	dates timeSlice
}

var data = AssetsData{}

// https://go.dev/tour/basics/15 | https://stackoverflow.com/a/22688926
const DbName = "binance_apr.sqlite"

type Period int

const (
	AllData Period = iota
	DayData
	WeekData
	MonthData
	YearData
)

func updateDataFromDB() {
	db, err := sql.Open("sqlite3", DbName)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	rows, err := db.Query("select time, asset, apy from apr")
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	// Обновляем объект CsvData новыми данными
	data.mu.Lock()
	defer data.mu.Unlock()
	data.dat = map[string]map[time.Time]float32{}
	dateSet := map[time.Time]struct{}{} // set: https://golang-blog.blogspot.com/2020/04/set-implementation-in-golang.html
	for rows.Next() {
		var dt time.Time
		var asset string
		var apy float32
		if err := rows.Scan(&dt, &asset, &apy); err != nil {
			log.Fatal(err)
		}
		// fmt.Printf("time=%s, asset=%s, apy=%f\n", dt.String(), asset, apy)
		if data.dat[asset] == nil {
			data.dat[asset] = map[time.Time]float32{}
		}
		data.dat[asset][dt] = apy
		dateSet[dt] = struct{}{}
	}

	// map sort https://www.geeksforgeeks.org/how-to-sort-golang-map-by-keys-or-values/
	// time sort https://www.socketloop.com/tutorials/golang-time-slice-or-date-sort-and-reverse-sort-example
	// zero length, capacity = len(dates)
	data.dates = make(timeSlice, 0, len(data.dates))
	for k := range dateSet {
		data.dates = append(data.dates, k)
	}
	sort.Sort(data.dates)
}

func updateDataFromDBLoop() {
	for {
		updateDataFromDB()
		// Обновляем содержимое файла раз в 5 минут
		time.Sleep(5 * time.Minute)
	}
}

func generateLineItems(dates timeSlice, vals map[time.Time]float32) []opts.LineData {
	items := make([]opts.LineData, len(dates))
	for i, t := range dates {
		if v, found := vals[t]; found {
			items[i] = opts.LineData{Value: v}
		}
	}
	return items
}

func makeLineChart(dataPeriod Period) *charts.Line {
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

	var minDt time.Time
	var now = time.Now()
	switch dataPeriod {
	case DayData:
		minDt = now.AddDate(0, 0, -1)
	case WeekData:
		minDt = now.AddDate(0, 0, -7)
	case MonthData:
		minDt = now.AddDate(0, -1, 0)
	case YearData:
		minDt = now.AddDate(-1, 0, 0)
	}
	var firstIndex int = -1
	for i, d := range data.dates {
		if d.Sub(minDt) >= 0 {
			firstIndex = i
			break
		}
	}
	if firstIndex == -1 {
		return line
	}

	var dates = data.dates[firstIndex:]
	// Put data into instance
	line.SetXAxis(dates).SetSeriesOptions(
		charts.WithLineChartOpts(opts.LineChart{Smooth: true}),
		// charts.WithLabelOpts(opts.Label{Show: true}),
	)
	for asset, vals := range data.dat {
		line.AddSeries(asset, generateLineItems(dates, vals))
	}
	return line
}

func endpointDay(w http.ResponseWriter, _ *http.Request) {
	makeLineChart(DayData).Render(w)
}

func endpointWeek(w http.ResponseWriter, _ *http.Request) {
	makeLineChart(WeekData).Render(w)
}

func endpointMonth(w http.ResponseWriter, _ *http.Request) {
	makeLineChart(MonthData).Render(w)
}

func endpointYear(w http.ResponseWriter, _ *http.Request) {
	makeLineChart(YearData).Render(w)
}

func endpointAllData(w http.ResponseWriter, _ *http.Request) {
	makeLineChart(AllData).Render(w)
}

func convert_csv_to_sqlite() {
	file, err := os.Open("binance_apr.csv")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		panic(err)
	}

	db, err := sql.Open("sqlite3", DbName)
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

func showRecords(recNum int) {
	db, err := sql.Open("sqlite3", DbName)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	rows, err := db.Query("select time, asset, apy, bonus from apr order by time desc limit ?", recNum)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	fmt.Println("Time\t\t\t\tAsset\tAPY\t\tBonus")
	for rows.Next() {
		var dt time.Time
		var asset string
		var apy float32
		var bonus float32
		if err := rows.Scan(&dt, &asset, &apy, &bonus); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\t%s\t%f\t%f\n", dt.String(), asset, apy, bonus)
	}
}

func main() {
	convFlag := flag.Bool("convert", false, "Convert CSV to SQLite")
	showFlag := flag.Int("show", 0, "Show last n records from database")
	flag.Parse()
	flag.Visit(func(f *flag.Flag) {
		// https://stackoverflow.com/a/54747682
		if f.Name == "convert" && *convFlag {
			println("Conversion...")
			convert_csv_to_sqlite()
			os.Exit(0)
		}
		if f.Name == "show" {
			if *showFlag <= 0 {
				panic("show: wrong number of records")
			}
			println("Database content:")
			showRecords(*showFlag)
			os.Exit(0)
		}
	})

	go updateDataFromDBLoop()

	http.HandleFunc("/", endpointWeek)
	http.HandleFunc("/day", endpointDay)
	http.HandleFunc("/week", endpointWeek)
	http.HandleFunc("/month", endpointMonth)
	http.HandleFunc("/year", endpointYear)
	http.HandleFunc("/all", endpointAllData)
	// Open port in firewall https://linuxconfig.org/how-to-allow-port-through-firewall-on-almalinux
	// firewall-cmd --zone=public --add-port 8081/tcp --permanent
	// firewall-cmd --reload
	// firewall-cmd --zone=public --list-ports
	http.ListenAndServe(":8081", nil)
}
