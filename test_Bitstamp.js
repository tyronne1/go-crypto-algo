// Make a POST request to the server to retrieve the account balance
function getBalance() {
    fetch('http://localhost:8080/balance', {
      method: 'POST'
    })
      .then(response => response.json())
      .then(data => {
        // Display the account balance on the web page
        document.getElementById('balance').innerHTML = data.balance;
      })
      .catch(error => {
        console.error('Error retrieving account balance:', error);
      });
  }
  