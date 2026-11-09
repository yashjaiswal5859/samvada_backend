document.addEventListener('DOMContentLoaded', () => {
  const otpSubtitle = document.getElementById('otp-subtitle');
  const errorBox = document.getElementById('error-box');
  const submitBtn = document.getElementById('btn-submit');
  const verifyContainer = document.getElementById('verify-container');
  const successContainer = document.getElementById('success-container');
  const dotVerify = document.getElementById('dot-verify');
  const dotSuccess = document.getElementById('dot-success');
  const countdownText = document.getElementById('countdown-text');

  // Extract requestId from path /api/v1/auth/verify/{requestId}
  const pathParts = window.location.pathname.split('/');
  const requestId = pathParts[pathParts.length - 1];

  // Extract parameters from URL
  const urlParams = new URLSearchParams(window.location.search);
  const loginChallenge = urlParams.get('login_challenge') || '';
  const mode = urlParams.get('mode') || 'login';

  // Display a generic OTP message — no email from localStorage
  otpSubtitle.innerHTML = `We've sent a verification code to your email.<br/><small style="color:var(--sr-primary);margin-top:6px;display:inline-block">💡 Look up OTP in Mailpit Web UI at http://localhost:8025</small>`;

  // Countdown timer
  let countdown = 42;
  let timerInterval = null;

  function startTimer() {
    countdown = 42;
    countdownText.innerHTML = `Resend OTP in ${countdown} seconds`;
    clearInterval(timerInterval);
    timerInterval = setInterval(() => {
      countdown--;
      if (countdown <= 0) {
        clearInterval(timerInterval);
        countdownText.innerHTML = `<span class="resend-btn" id="btn-resend">Resend verification code</span>`;
        document.getElementById('btn-resend').addEventListener('click', handleResend);
      } else {
        countdownText.innerHTML = `Resend OTP in ${countdown} seconds`;
      }
    }, 1000);
  }

  startTimer();

  // OTP inputs handling
  const otpBoxes = document.querySelectorAll('.otp-box');
  
  otpBoxes.forEach((box, idx) => {
    box.addEventListener('input', (e) => {
      box.value = box.value.replace(/[^0-9]/g, '');
      if (box.value.length === 1 && idx < 5) {
        otpBoxes[idx + 1].focus();
      }
    });

    box.addEventListener('keydown', (e) => {
      if (e.key === 'Backspace' && !box.value && idx > 0) {
        otpBoxes[idx - 1].focus();
      }
    });
  });

  // Paste handler
  document.querySelector('.otp-row').addEventListener('paste', (e) => {
    e.preventDefault();
    const text = (e.clipboardData || window.clipboardData).getData('text');
    const cleanText = text.replace(/[^0-9]/g, '').substring(0, 6);
    
    for (let i = 0; i < cleanText.length; i++) {
      otpBoxes[i].value = cleanText[i];
    }
    if (cleanText.length > 0) {
      const nextFocus = Math.min(cleanText.length, 5);
      otpBoxes[nextFocus].focus();
    }
  });

  // Resend OTP handler — no localStorage used
  async function handleResend() {
    errorBox.style.display = 'none';
    // Cannot resend without knowing the email — redirect to start of flow
    errorBox.textContent = 'Please start a new login from the login page.';
    errorBox.style.display = 'block';
  }

  // Submit Handler
  submitBtn.addEventListener('click', async () => {
    errorBox.style.display = 'none';
    
    let otp = '';
    otpBoxes.forEach(box => otp += box.value);

    if (otp.length !== 6) {
      errorBox.textContent = 'Please enter a valid 6-digit verification code.';
      errorBox.style.display = 'block';
      return;
    }

    submitBtn.disabled = true;
    submitBtn.innerHTML = `<span class="spinner"></span>`;

    const payload = {
      requestId,
      otp
    };
    if (loginChallenge) {
      payload.loginChallenge = loginChallenge;
    }

    try {
      const res = await fetch('/api/v1/auth/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });

      const data = await res.json();

      if (!res.ok || !data.success) {
        throw new Error(data.error?.message || 'Verification failed');
      }

      // Access Granted Transition
      verifyContainer.style.display = 'none';
      successContainer.style.display = 'block';
      dotVerify.classList.remove('active');
      dotVerify.classList.add('completed');
      dotSuccess.classList.add('active');

      setTimeout(() => {
        if (data.data && data.data.redirectTo) {
          // Validate redirect URL is same-origin before following
          try {
            const redirectUrl = new URL(data.data.redirectTo, window.location.origin);
            if (redirectUrl.origin === window.location.origin || redirectUrl.hostname === 'localhost') {
              window.location.href = data.data.redirectTo;
            } else {
              window.location.href = '/api/v1/auth/oauth/login';
            }
          } catch {
            // Relative URL — safe to follow
            window.location.href = data.data.redirectTo;
          }
        } else {
          window.location.href = '/api/v1/auth/oauth/login';
        }
      }, 1200);

    } catch (err) {
      const msg = err.message.toLowerCase();
      if (msg.includes('challenge') || msg.includes('not found') || msg.includes('used') || msg.includes('expire')) {
        errorBox.innerHTML = `Login session expired or invalid. <a href="/api/v1/auth/oauth/login" style="color: var(--sr-primary); text-decoration: underline;">Click here to start over</a>.`;
      } else {
        errorBox.textContent = err.message;
      }
      errorBox.style.display = 'block';
      submitBtn.disabled = false;
      submitBtn.innerHTML = 'Verify and login';
    }
  });

  // Focus on first box initially
  otpBoxes[0].focus();
});
