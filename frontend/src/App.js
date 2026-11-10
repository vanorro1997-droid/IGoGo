import React, { useState, useEffect } from 'react';

function App() {
  const [data, setData] = useState(null);
  const [health, setHealth] = useState(null);

  useEffect(() => {
    fetch('/api/health')
      .then(res => res.json())
      .then(data => setHealth(data))
      .catch(err => console.error(err));

    fetch('/api/data')
      .then(res => res.json())
      .then(data => setData(data))
      .catch(err => console.error(err));
  }, []);

  return (
    <div style={{ padding: '20px', fontFamily: 'Arial' }}>
      <h1>IGoGo Application</h1>
      <h2>Health Status</h2>
      <pre>{JSON.stringify(health, null, 2)}</pre>
      <h2>API Data</h2>
      <pre>{JSON.stringify(data, null, 2)}</pre>
    </div>
  );
}

export default App;
