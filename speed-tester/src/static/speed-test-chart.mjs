const ctx = document.querySelector('.js-speed-test-chart').getContext('2d');

const data = Array.from(document.querySelectorAll('.js-test-result'))
  .map(r => ({
    t: new Date(r.dataset.date),
    y: parseFloat(r.dataset.download)
  }));

data.reverse();

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

console.log(chartParameters);

const chart = new Chart(ctx, chartParameters);