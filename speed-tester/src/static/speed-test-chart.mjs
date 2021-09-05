const SITE_ROOT = document.documentElement.dataset.siteRoot

buildCharts();

async function buildCharts() {

  {
    const dayCtx = document.querySelector('.js-speed-test-chart--day').getContext('2d');
    const data = Array.from(document.querySelectorAll('.js-test-result'))
      .map(r => ({
        t: new Date(r.dataset.date),
        y: parseFloat(r.dataset.download)
      }));

    data.reverse();

    generateChart(data, dayCtx);
  }

  {
    const monthCtx = document.querySelector('.js-speed-test-chart--month').getContext('2d');
    const response = await fetch(`${SITE_ROOT}/history/lastMonth/`);
    const data = await response.json();
    const mapped = data.map(r => ({
      t: new Date(r.Time),
      y: parseFloat(r.DownloadSpeed)
    }));

    mapped.sort((a, b) => a.t - b.t);

    generateChart(mapped, monthCtx);
  }


  {
    const yearCtx = document.querySelector('.js-speed-test-chart--year').getContext('2d');
    const response = await fetch(`${SITE_ROOT}/history/lastYear/`);
    const data = await response.json();
    const mapped = data.map(r => ({
      t: new Date(r.Time),
      y: parseFloat(r.DownloadSpeed)
    }));

    mapped.sort((a, b) => a.t - b.t);

    generateChart(mapped, yearCtx);
  }


}

async function generateChart(data, context) {
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

