document.addEventListener('DOMContentLoaded', () => {
  const form = document.getElementById('login-form');
  const emailInput = document.getElementById('email');
  const submitBtn = document.getElementById('btn-submit');
  const errorBox = document.getElementById('error-box');
  const linkRegister = document.getElementById('link-register');

  // Extract challenge from URL (passed by backend in the redirect)
  const urlParams = new URLSearchParams(window.location.search);
  const loginChallenge = urlParams.get('login_challenge') || '';

  // Link routing
  if (linkRegister) {
    linkRegister.addEventListener('click', (e) => {
      e.preventDefault();
      window.location.href = `/api/v1/auth/register-challenge?login_challenge=${encodeURIComponent(loginChallenge)}`;
    });
  }

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    errorBox.style.display = 'none';

    const email = emailInput.value.trim();
    if (!email) return;

    // Set loading
    submitBtn.disabled = true;
    submitBtn.innerHTML = `<span class="spinner"></span>`;

    try {
      const res = await fetch('/api/v1/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email })
      });

      const data = await res.json();

      if (!res.ok || !data.success) {
        throw new Error(data.error?.message || 'Failed to request verification code');
      }

      const requestId = data.data.requestId;

      // Redirect to unified RESTful verify page
      // Note: email is NOT stored in localStorage for security (Loophole 12 fix).
      // The session_id cookie binds this flow to the browser automatically.
      window.location.href = `/api/v1/auth/verify/${requestId}?login_challenge=${encodeURIComponent(loginChallenge)}&mode=login`;
    } catch (err) {
      errorBox.textContent = err.message;
      errorBox.style.display = 'block';
      submitBtn.disabled = false;
      submitBtn.innerHTML = 'Request Verification Code';
    }
  });
});
