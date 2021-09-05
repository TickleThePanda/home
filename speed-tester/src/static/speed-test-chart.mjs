const SITE_ROOT = document.documentElement.dataset.siteRoot

buildCharts();

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

        return data;
      },
      unit: 'hour'
    },
    {
      context: document.querySelector('.js-speed-test-chart--month').getContext('2d'),
      data: async () => {
        const response = await fetch(`${SITE_ROOT}/history/lastMonth/`);
        const data = await response.json();
        const mapped = data.map(r => ({
          t: new Date(r.Time),
          y: parseFloat(r.DownloadSpeed)
        }));

        mapped.sort((a, b) => a.t - b.t);

        return mapped;
      },
      unit: 'day'
    },
    {
      context: document.querySelector('.js-speed-test-chart--year').getContext('2d'),
      data: async () => {
        const response = await fetch(`${SITE_ROOT}/history/lastYear/`);
        const data = await response.json();
        const mapped = data.map(r => ({
          t: new Date(r.Time),
          y: parseFloat(r.DownloadSpeed)
        }));

        mapped.sort((a, b) => a.t - b.t);

        return mapped;
      },
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
  const chartParameters = {
    type: 'line',
    data: {
      datasets: [{
        data: data,
        color: '#ccc',
        backgroundColor: '#ccc',
        pointBorderColor: '#ccc',
        borderColor: '#888',
        borderWidth: 2,
        cubicInterpolationMode: 'monotone',
        fill: false
      }]
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

