package main

import (
	"encoding/csv"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"
)

type CsvData struct {
	mu      sync.RWMutex
	records [][]string
	dates   []string
	dat     map[string][]float64
}

var file *os.File
var reader *csv.Reader
var data *CsvData

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

func generateLineItems(vals []float64) []opts.LineData {
	items := make([]opts.LineData, 0)
	for i := 0; i < len(vals); i++ {
		items = append(items, opts.LineData{Value: vals[i]})
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
	line.SetXAxis(data.dates).SetSeriesOptions(
		charts.WithLineChartOpts(opts.LineChart{Smooth: true}),
		// charts.WithLabelOpts(opts.Label{Show: true}),
	)
	for asset, vals := range data.dat {
		line.AddSeries(asset, generateLineItems(vals))
	}
	line.Render(w)
}

func main() {
	file, err := os.Open("binance_apr.csv")
	if err != nil {
		panic(err)
	}
	defer file.Close()
	reader = csv.NewReader(file)
	data = &CsvData{}

	go updateDataFromCSV()

	http.HandleFunc("/", httpserver)
	// Open port in firewall https://linuxconfig.org/how-to-allow-port-through-firewall-on-almalinux
	// firewall-cmd --zone=public --add-port 8081/tcp --permanent
	// firewall-cmd --reload
	// firewall-cmd --zone=public --list-ports
	http.ListenAndServe(":8081", nil)
}
