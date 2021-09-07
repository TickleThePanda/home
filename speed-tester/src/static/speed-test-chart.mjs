const SITE_ROOT = document.documentElement.dataset.siteRoot

buildCharts();

async function fetchDatasetFromUrl(url) {
  const response = await fetch(url);
  const data = await response.json();
  const median = data.map(r => ({
    t: new Date(r.Time),
    y: parseFloat(r.DownloadSpeedMedian)
  }));

  const p90th = data.map(r => ({
    t: new Date(r.Time),
    y: parseFloat(r.DownloadSpeed90th)
  }));

  median.sort((a, b) => a.t - b.t);
  p90th.sort((a, b) => a.t - b.t);

  return {median, p90th}
}

async function buildCharts() {

  const charts = [
    {
      context: document.querySelector('.js-speed-test-chart--day').getContext('2d'),
      data: () => {
        const data = Array.from(document.querySelectorAll('.js-test-result'))
          .map(r => ({
            t: new Date(r.dataset.date),
            y: parseFloat(r.dataset.download)
          }));

        data.reverse();

        return { median: data };
      },
      unit: 'hour'
    },
    {
      context: document.querySelector('.js-speed-test-chart--month').getContext('2d'),
      data: async () => await fetchDatasetFromUrl(`${SITE_ROOT}/history/lastMonth/`),
      unit: 'day'
    },
    {
      context: document.querySelector('.js-speed-test-chart--year').getContext('2d'),
      data: async () => await fetchDatasetFromUrl(`${SITE_ROOT}/history/lastYear/`),
      unit: 'month'
    }

  ]

  charts.forEach(async (c) => {
    const data = await c.data();
    generateChart({
      data,
      context: c.context,
      unit: c.unit
    });
  })
}

async function generateChart({
  data,
  unit,
  context
}) {

  const datasets = [];

  if (data.median) {
    datasets.push(({
      data: data.median,
      color: '#ccc',
      backgroundColor: '#ccc',
      pointBorderColor: '#ccc',
      borderColor: '#888',
      borderWidth: 2,
      cubicInterpolationMode: 'monotone',
      fill: false
    }));
  }

  if (data.p90th) {
    datasets.push(({
      data: data.p90th,
      color: '#daa',
      backgroundColor: '#daa',
      pointBorderColor: '#daa',
      borderColor: '#888',
      borderWidth: 2,
      cubicInterpolationMode: 'monotone',
      fill: false
    }));
  }

  const chartParameters = {
    type: 'line',
    data: {
      datasets
    },
    options: {
      legend: {
        display: false
      },
      scales: {
        xAxes: [{
          type: 'time',
          gridLines: {
            drawOnChartArea: false,
            color: '#888'
          },
          ticks: {
            fontColor: '#eee'
          },
          time: {
            unit
          }
        }],
        yAxes: [{
          type: 'linear',
          gridLines: {
            drawOnChartArea: false,
            color: '#888'
          },
          ticks: {
            fontColor: '#eee'
          }
        }]
      },
    }
  };

  const chart = new Chart(context, chartParameters);
}

