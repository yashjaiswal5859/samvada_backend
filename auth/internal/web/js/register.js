document.addEventListener('DOMContentLoaded', () => {
  const form = document.getElementById('register-form');
  const emailInput = document.getElementById('email');
  const deviceIdInput = document.getElementById('deviceId');
  const displayNameInput = document.getElementById('displayName');
  const phoneNumberInput = document.getElementById('phoneNumber');
  const submitBtn = document.getElementById('btn-submit');
  const errorBox = document.getElementById('error-box');
  const linkLogin = document.getElementById('link-login');

  // Extract challenge
  const urlParams = new URLSearchParams(window.location.search);
  const loginChallenge = urlParams.get('login_challenge') || '';

  // Link routing
  if (linkLogin) {
    linkLogin.addEventListener('click', (e) => {
      e.preventDefault();
      window.location.href = `/api/v1/auth/login-challenge?login_challenge=${encodeURIComponent(loginChallenge)}`;
    });
  }

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    errorBox.style.display = 'none';

    const email = emailInput.value.trim();
    const deviceId = deviceIdInput.value.trim();
    const displayName = displayNameInput.value.trim();
    const phoneNumber = phoneNumberInput.value.trim();

    if (!email || !deviceId || !displayName) return;

    // Set loading
    submitBtn.disabled = true;
    submitBtn.innerHTML = `<span class="spinner"></span>`;

    const payload = {
      email,
      deviceId,
      displayName
    };
    if (phoneNumber) {
      payload.phoneNumber = phoneNumber;
    }

    try {
      const res = await fetch('/api/v1/auth/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });

      const data = await res.json();

      if (!res.ok || !data.success) {
        throw new Error(data.error?.message || 'Failed to request registration code');
      }

      const requestId = data.data.requestId;
      
      // Store in localStorage for verify phase resending/display
      localStorage.setItem('saferide_draft_email', email);
      localStorage.setItem('saferide_draft_mode', 'register');
      localStorage.setItem('saferide_draft_device_id', deviceId);
      localStorage.setItem('saferide_draft_display_name', displayName);
      if (phoneNumber) {
        localStorage.setItem('saferide_draft_phone', phoneNumber);
      } else {
        localStorage.removeItem('saferide_draft_phone');
      }

      // Redirect to unified RESTful verify page
      window.location.href = `/api/v1/auth/verify/${requestId}?login_challenge=${encodeURIComponent(loginChallenge)}`;
    } catch (err) {
      errorBox.textContent = err.message;
      errorBox.style.display = 'block';
      submitBtn.disabled = false;
      submitBtn.innerHTML = 'Request Verification Code';
    }
  });
});
