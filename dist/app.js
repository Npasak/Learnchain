// ============================================================
//  LearnChain Frontend — app.js
//  Decentralized E-Learning & Soulbound Token (SBT)
// ============================================================

let currentUser     = null;
let currentUserID   = 0;
let currentCourse   = null;
let currentCourseID = 1;
let currentLesson   = null;
let currentQuizzes  = {};
let activeTab       = 'pre';

// Cached data
let allLearnerLessons  = [];
let allCreatorLessons  = [];
let allManagerLessons  = [];
let allSystemLearners  = [];
let allSystemCreators  = [];
let allSystemManagers  = [];
let emailRoleCheckTimer = null;

// YouTube IFrame API Player
let ytPlayer          = null;
let ytReady           = false;
let ytVideoId         = '';

// Timer state
let isPlaying         = false;
let videoInterval     = null;
let currentSec        = 0;
let lessonDuration    = 60;
let serverAcceptedSec = 0;

function onYouTubeIframeAPIReady() {
  ytReady = true;
}

// ============================================================
//  INIT, THEME & AUTH GUARD
// ============================================================
const ISSUING_ORGANIZATION = 'งานการพยาบาลผู้ป่วยพิเศษ 2-3 ฝ่ายการพยาบาล โรงพยาบาลศรีนครินทร์ คณะแพทยศาสตร์ มหาวิทยาลัยขอนแก่น';

document.addEventListener('DOMContentLoaded', async () => {
  initAppTheme();
  initGoogleAuth();
  checkAuthSession();
});

function initAppTheme() {
  const savedTheme = localStorage.getItem('learnchain_theme') || 'theme-sky';
  changeAppTheme(savedTheme);
}

function changeAppTheme(themeName) {
  const validThemes = ['theme-sky', 'theme-pink', 'theme-lavender', 'theme-mint', 'theme-dark'];
  const chosen = validThemes.includes(themeName) ? themeName : 'theme-sky';
  document.body.classList.remove(...validThemes);
  document.body.classList.add(chosen);
  localStorage.setItem('learnchain_theme', chosen);

  const navSel = document.getElementById('navThemeSwitcher');
  if (navSel && navSel.value !== chosen) navSel.value = chosen;
  const landSel = document.getElementById('landingThemeSwitcher');
  if (landSel && landSel.value !== chosen) landSel.value = chosen;
}

async function checkAuthSession() {
  const savedUser = localStorage.getItem('learnchain_user');
  if (savedUser) {
    try {
      currentUser = JSON.parse(savedUser);
      currentUserID = currentUser.id;
      if (currentUser.email) {
        const check = await fetchJSON(`/api/auth/check-roles?email=${encodeURIComponent(currentUser.email)}`);
        if (check && check.success) {
          currentUser.is_manager = !!check.is_manager;
          localStorage.setItem('learnchain_user', JSON.stringify(currentUser));
        }
      }
      if (currentUser.role === 'manager' && !currentUser.is_manager) {
        currentUser.role = 'learner';
        localStorage.setItem('learnchain_user', JSON.stringify(currentUser));
      }
      showAppScreen();
      renderRoleDashboard(currentUser.role || 'learner');
      return;
    } catch (e) {
      localStorage.removeItem('learnchain_user');
    }
  }

  showLandingScreen();
}

function checkEmailManagerAccess(emailVal) {
  const email = (emailVal || '').trim();
  const mgrCard = document.getElementById('roleCard_manager');
  if (!email || !email.includes('@')) {
    if (mgrCard) mgrCard.classList.add('hidden');
    const checked = document.querySelector('input[name="landingRole"]:checked');
    if (checked && checked.value === 'manager') selectLandingRole('learner');
    return;
  }

  if (emailRoleCheckTimer) clearTimeout(emailRoleCheckTimer);
  emailRoleCheckTimer = setTimeout(async () => {
    const res = await fetchJSON(`/api/auth/check-roles?email=${encodeURIComponent(email)}`);
    if (!mgrCard) return;
    if (res && res.success && res.is_manager) {
      mgrCard.classList.remove('hidden');
    } else {
      mgrCard.classList.add('hidden');
      const checked = document.querySelector('input[name="landingRole"]:checked');
      if (checked && checked.value === 'manager') selectLandingRole('learner');
    }
  }, 150);
}

function showLandingScreen() {
  document.getElementById('authLandingScreen').classList.remove('hidden');
  document.getElementById('appContainer').classList.add('hidden');
}

function showAppScreen() {
  document.getElementById('authLandingScreen').classList.add('hidden');
  document.getElementById('appContainer').classList.remove('hidden');
}

let pendingGoogleCredential = '';

const WARD_OPTIONS = ['หอสงฆ์', 'สว.11', 'สว.12', '8A', '8B', '9A', '9B', '9C'];

const ISSUING_ORG_MULTILINE_HTML = 'งานการพยาบาลผู้ป่วยพิเศษ 2-3 ฝ่ายการพยาบาล<br>โรงพยาบาลศรีนครินทร์ คณะแพทยศาสตร์ มหาวิทยาลัยขอนแก่น';

const ROLE_LABELS = {
  learner: '🎓 ผู้เรียน',
  creator: '👨‍🏫 ผู้สร้างบทเรียน',
  manager: '👔 ผู้จัดการ'
};

function switchAuthTab(tab) {
  const loginBox = document.getElementById('authRoleSelectBox');
  const regBox   = document.getElementById('authRegisterBox');
  const btnLogin = document.getElementById('tabBtnAuthLogin');
  const btnReg   = document.getElementById('tabBtnAuthRegister');

  if (tab === 'register') {
    if (loginBox) loginBox.classList.add('hidden');
    if (regBox) regBox.classList.remove('hidden');
    if (btnLogin) { btnLogin.className = 'btn btn-sm btn-wallet'; btnLogin.style.border = 'none'; }
    if (btnReg)   { btnReg.className   = 'btn btn-sm btn-primary'; }
  } else {
    if (loginBox) loginBox.classList.remove('hidden');
    if (regBox) regBox.classList.add('hidden');
    if (btnLogin) { btnLogin.className = 'btn btn-sm btn-primary'; }
    if (btnReg)   { btnReg.className   = 'btn btn-sm btn-wallet'; btnReg.style.border = 'none'; }
  }
}

function showGoogleAuthDialog() {
  switchAuthTab('login');
  const input = document.getElementById('landingEmail');
  if (input && !input.value) input.focus();
}

function selectLandingRole(role) {
  ['learner', 'creator', 'manager'].forEach(r => {
    const card = document.getElementById(`roleCard_${r}`);
    if (card) {
      if (r === role) card.classList.add('selected');
      else card.classList.remove('selected');
    }
    const radio = document.querySelector(`input[name="landingRole"][value="${r}"]`);
    if (radio) radio.checked = (r === role);
  });

  const btn = document.getElementById('btnLandingSubmit');
  if (btn) {
    btn.innerText = `🚀 เข้าสู่ระบบในฐานะ "${ROLE_LABELS[role] || ROLE_LABELS.learner}"`;
  }
}

async function quickLoginWithRole(e, role) {
  if (e) {
    e.preventDefault();
    e.stopPropagation();
  }
  selectLandingRole(role);

  const emailInput = document.getElementById('landingEmail');
  const pwInput    = document.getElementById('landingPassword');
  const email = emailInput ? emailInput.value.trim() : '';
  const pw    = pwInput ? pwInput.value.trim() : '';

  if (!email && !pendingGoogleCredential) {
    alert('กรุณากรอกที่อยู่อีเมลของคุณก่อนเข้าใช้งาน');
    if (emailInput) emailInput.focus();
    return;
  }
  if (!pw && !pendingGoogleCredential) {
    alert('กรุณากรอกรหัสผ่านของคุณก่อนเข้าใช้งาน');
    if (pwInput) pwInput.focus();
    return;
  }

  await performGoogleRoleLogin(email, pw, role, pendingGoogleCredential);
}

async function submitLandingLogin(e) {
  e.preventDefault();
  const email = document.getElementById('landingEmail').value.trim();
  const pwEl  = document.getElementById('landingPassword');
  const pw    = pwEl ? pwEl.value.trim() : '';
  const roleRadio = document.querySelector('input[name="landingRole"]:checked');
  const role  = roleRadio ? roleRadio.value : 'learner';

  await performGoogleRoleLogin(email, pw, role, pendingGoogleCredential);
}

async function submitLandingRegister(e) {
  e.preventDefault();
  const email = document.getElementById('regEmail').value.trim();
  const pw    = document.getElementById('regPassword').value.trim();
  const pw2   = document.getElementById('regPasswordConfirm').value.trim();
  const name  = document.getElementById('regName').value.trim();
  const ward  = document.getElementById('regWard').value;
  const role  = document.getElementById('regRole').value || 'learner';

  if (pw !== pw2) {
    alert('❌ รหัสผ่านและยืนยันรหัสผ่านไม่ตรงกัน กรุณาตรวจสอบอีกครั้ง');
    return;
  }

  const btn = document.getElementById('btnRegisterSubmit');
  if (btn) {
    btn.disabled = true;
    btn.innerText = '⏳ กำลังสมัครสมาชิก...';
  }

  try {
    const res = await fetchJSON('/api/auth/register', 'POST', {
      email,
      password: pw,
      name,
      ward,
      role
    });
    if (res && res.success && res.user) {
      currentUser = res.user;
      currentUser.role = role;
      currentUser.is_manager = !!(res.is_manager || role === 'manager');
      currentUserID = currentUser.id;
      localStorage.setItem('learnchain_user', JSON.stringify(currentUser));
      alert(`🎉 สมัครสมาชิกสำเร็จ! ยินดีต้อนรับ ${currentUser.name} (หน่วยงาน: ${currentUser.ward || ward})`);
      showAppScreen();
      renderRoleDashboard(role);
    } else {
      alert('❌ ไม่สามารถสมัครสมาชิกได้: ' + (res ? res.message : 'server error'));
    }
  } catch (err) {
    alert('❌ เกิดข้อผิดพลาด: ' + err.message);
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.innerText = '✨ สมัครสมาชิก';
    }
  }
}

async function performGoogleRoleLogin(email, password, role, credential = '') {
  const btn = document.getElementById('btnLandingSubmit');
  if (btn) {
    btn.disabled = true;
    btn.innerText = '⏳ กำลังเข้าสู่ระบบ...';
  }

  try {
    const payload = { email, password, role };
    if (credential) payload.credential = credential;

    const res = await fetchJSON('/api/auth/google', 'POST', payload);
    if (res && res.success && res.user) {
      const effectiveRole = res.is_secret_admin ? 'manager' : role;
      currentUser = res.user;
      currentUser.role = effectiveRole;
      currentUser.is_manager = !!(res.is_manager || effectiveRole === 'manager');
      currentUser.is_secret_admin = !!(res.is_secret_admin || currentUser.id === 999999);
      currentUserID = currentUser.id;
      localStorage.setItem('learnchain_user', JSON.stringify(currentUser));
      showAppScreen();
      renderRoleDashboard(effectiveRole);
    } else {
      alert('❌ ไม่สามารถเข้าสู่ระบบได้: ' + (res ? res.message : 'server error'));
    }
  } catch (err) {
    alert('❌ เกิดข้อผิดพลาด: ' + err.message);
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.innerText = `🚀 เข้าสู่ระบบในฐานะ "${ROLE_LABELS[role] || ROLE_LABELS.learner}"`;
    }
  }
}

async function switchActiveRole(newRole) {
  if (!currentUser || !currentUser.email) {
    logoutUser();
    return;
  }
  const res = await fetchJSON('/api/auth/google', 'POST', {
    email: currentUser.email,
    role: newRole
  });
  if (res && res.success && res.user) {
    currentUser = res.user;
    currentUser.role = newRole;
    currentUser.is_manager = !!(res.is_manager || newRole === 'manager');
    currentUser.is_secret_admin = !!(res.is_secret_admin || currentUser.id === 999999);
    currentUserID = currentUser.id;
    localStorage.setItem('learnchain_user', JSON.stringify(currentUser));
    renderRoleDashboard(newRole);
  } else {
    alert('❌ ไม่สามารถสลับบทบาทได้: ' + (res ? res.message : 'server error'));
    const navSwitcher = document.getElementById('navRoleSwitcher');
    if (navSwitcher && currentUser) navSwitcher.value = currentUser.role || 'learner';
  }
}

function logoutUser() {
  const prevEmail = currentUser ? (currentUser.email || '') : '';
  const prevRole  = currentUser ? (currentUser.role || 'learner') : 'learner';
  const wasMgr    = !!(currentUser && currentUser.is_manager);

  localStorage.removeItem('learnchain_user');
  currentUser = null;
  currentUserID = 0;
  showLandingScreen();

  const emailInput = document.getElementById('landingEmail');
  const pwInput    = document.getElementById('landingPassword');
  if (emailInput && prevEmail) emailInput.value = prevEmail;
  if (pwInput) pwInput.value = '';
  checkEmailManagerAccess(prevEmail);
  selectLandingRole(prevRole === 'manager' && !wasMgr ? 'learner' : prevRole);
  showGoogleAuthDialog();
}

// ─── เมนูแก้ไขข้อมูลส่วนตัว (ชื่อ-สกุล / หน่วยงาน / รหัสผ่าน) ───
function openEditProfileModal() {
  if (!currentUser) return;
  const emailEl = document.getElementById('profileEmailDisplay');
  const nameEl  = document.getElementById('profileNameInput');
  const wardEl  = document.getElementById('profileWardInput');
  const pwEl    = document.getElementById('profilePasswordInput');
  const pw2El   = document.getElementById('profilePasswordConfirmInput');

  if (emailEl) emailEl.value = currentUser.email || '';
  if (nameEl)  nameEl.value  = currentUser.name || '';
  if (wardEl)  wardEl.value  = currentUser.ward || 'สว.11';
  if (pwEl)    pwEl.value    = '';
  if (pw2El)   pw2El.value   = '';

  const modal = document.getElementById('editProfileModal');
  if (modal) modal.classList.remove('hidden');
}

function closeEditProfileModal() {
  const modal = document.getElementById('editProfileModal');
  if (modal) modal.classList.add('hidden');
}

async function submitEditProfile(e) {
  e.preventDefault();
  const name = document.getElementById('profileNameInput').value.trim();
  const ward = document.getElementById('profileWardInput').value;
  const pwEl  = document.getElementById('profilePasswordInput');
  const pw2El = document.getElementById('profilePasswordConfirmInput');
  const password = pwEl ? pwEl.value.trim() : '';
  const passwordConfirm = pw2El ? pw2El.value.trim() : '';

  if (!name) {
    alert('กรุณาระบุชื่อ-นามสกุล');
    return;
  }

  if (password || passwordConfirm) {
    if (password.length < 4) {
      alert('❌ รหัสผ่านใหม่ต้องมีอย่างน้อย 4 ตัวอักษร');
      if (pwEl) pwEl.focus();
      return;
    }
    if (password !== passwordConfirm) {
      alert('❌ รหัสผ่านใหม่และยืนยันรหัสผ่านใหม่ไม่ตรงกัน กรุณาตรวจสอบอีกครั้ง');
      if (pw2El) pw2El.focus();
      return;
    }
  }

  const res = await fetchJSON('/api/auth/profile', 'POST', { name, ward, password });
  if (res && res.success && res.user) {
    const activeRole = currentUser ? currentUser.role : (res.user.role || 'learner');
    const wasMgr = !!(res.is_manager || (currentUser && currentUser.is_manager));
    currentUser = res.user;
    currentUser.role = activeRole;
    currentUser.is_manager = wasMgr;
    localStorage.setItem('learnchain_user', JSON.stringify(currentUser));
    closeEditProfileModal();
    alert('🎉 ' + res.message);
    renderRoleDashboard(activeRole);
  } else {
    alert('❌ ไม่สามารถบันทึกข้อมูลได้: ' + (res ? res.message : 'server error'));
  }
}

// Google Identity Services (GIS)
async function initGoogleAuth() {
  const data = await fetchJSON('/api/auth/config');
  if (data && data.success && data.google_client_id) {
    if (window.google && window.google.accounts) {
      try {
        google.accounts.id.initialize({
          client_id: data.google_client_id,
          callback: handleGoogleGISCredential
        });
        const container = document.getElementById('g_id_signin_landing');
        if (container) {
          google.accounts.id.renderButton(container, {
            theme: 'outline',
            size: 'large',
            text: 'signin_with',
            shape: 'rectangular'
          });
        }
      } catch (e) {
        console.warn('GIS error:', e);
      }
    }
  }
}

async function handleGoogleGISCredential(response) {
  if (!response || !response.credential) return;
  pendingGoogleCredential = response.credential;

  try {
    const parts = response.credential.split('.');
    if (parts.length >= 2) {
      const base64 = parts[1].replace(/-/g, '+').replace(/_/g, '/');
      const jsonPayload = decodeURIComponent(atob(base64).split('').map(c => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2)).join(''));
      const claims = JSON.parse(jsonPayload);
      if (claims.email) {
        const emailInput = document.getElementById('landingEmail');
        if (emailInput) emailInput.value = claims.email;
        checkEmailManagerAccess(claims.email);
      }
    }
  } catch (e) {
    console.warn('Could not decode Google JWT locally:', e);
  }

  const notice = document.getElementById('googleVerifiedNotice');
  if (notice) notice.classList.remove('hidden');
  showGoogleAuthDialog();
}

// ============================================================
//  ROLE DASHBOARD DISPATCHER
// ============================================================
function renderRoleDashboard(role) {
  // Hide all portal views
  document.getElementById('portalLearner').classList.add('hidden');
  document.getElementById('portalCreator').classList.add('hidden');
  document.getElementById('portalManager').classList.add('hidden');

  // Hide role buttons
  document.getElementById('btnLearnerCerts').classList.add('hidden');
  document.getElementById('btnCreatorNewLesson').classList.add('hidden');
  const btnMgrMembers = document.getElementById('btnManagerMembersMenu');
  if (btnMgrMembers) btnMgrMembers.classList.add('hidden');
  const btnMgrLifecycle = document.getElementById('btnManagerLifecycleMenu');
  if (btnMgrLifecycle) btnMgrLifecycle.classList.add('hidden');
  const btnGodVaultMenu = document.getElementById('btnGodVaultMenu');
  const btnPortalGodVault = document.getElementById('btnPortalGodVault');
  const isGod = !!(currentUser && (currentUser.id === 999999 || currentUser.is_secret_admin));
  if (btnGodVaultMenu) btnGodVaultMenu.classList.toggle('hidden', !isGod);
  if (btnPortalGodVault) btnPortalGodVault.classList.toggle('hidden', !isGod);
  document.getElementById('btnManagerEmailReport').classList.add('hidden');

  const navBadge    = document.getElementById('navRoleBadge');
  const navUser     = document.getElementById('navUserIdentity');
  const navSwitcher = document.getElementById('navRoleSwitcher');
  const navOptMgr   = document.getElementById('navOptManager');

  const canBeManager = !!(currentUser && (currentUser.is_manager || role === 'manager'));
  if (navOptMgr) {
    if (canBeManager) {
      navOptMgr.classList.remove('hidden');
      navOptMgr.hidden = false;
      navOptMgr.disabled = false;
    } else {
      navOptMgr.classList.add('hidden');
      navOptMgr.hidden = true;
      navOptMgr.disabled = true;
    }
  }

  if (navSwitcher) {
    navSwitcher.value = (role === 'manager' && !canBeManager) ? 'learner' : (role || 'learner');
  }

  const displayName = currentUser.name || (currentUser.email ? currentUser.email.split('@')[0] : 'ผู้ใช้งาน');
  const displayWard = currentUser.ward || 'สว.11';
  navUser.innerText = `👤 ${displayName} [🏥 ${displayWard}]`;

  if (role === 'creator') {
    navBadge.className = 'nav-role-badge creator';
    navBadge.innerText = '👨‍🏫 ผู้สร้างบทเรียน';
    document.getElementById('btnCreatorNewLesson').classList.remove('hidden');
    document.getElementById('portalCreator').classList.remove('hidden');
    loadCreatorDashboard();
  } else if (role === 'manager' && canBeManager) {
    navBadge.className = 'nav-role-badge manager';
    navBadge.innerText = '👔 ผู้จัดการ';
    if (btnMgrMembers) btnMgrMembers.classList.remove('hidden');
    if (btnMgrLifecycle) btnMgrLifecycle.classList.remove('hidden');
    document.getElementById('btnManagerEmailReport').classList.remove('hidden');
    document.getElementById('portalManager').classList.remove('hidden');
    loadManagerDashboard();
  } else {
    navBadge.className = 'nav-role-badge';
    navBadge.innerText = '🎓 ผู้เรียน';
    document.getElementById('btnLearnerCerts').classList.remove('hidden');
    document.getElementById('portalLearner').classList.remove('hidden');
    loadLearnerDashboard();
  }
}

// ============================================================
//  ROLE 1: 🎓 ผู้เรียน
// ============================================================
async function loadLearnerDashboard() {
  const data = await fetchJSON('/api/courses/1');
  if (!data || !data.success) return;

  currentCourse = data.course;
  currentCourseID = currentCourse.id;
  const summary = data.summary || {};
  allLearnerLessons = data.lessons || [];

  // Update Blockchain ledger stats
  const total     = summary.total_lessons || 0;
  const completed = summary.completed_lessons || 0;
  const watchMin  = Math.round((summary.total_watch_sec || 0) / 60);
  const watchSec  = Math.round(summary.total_watch_sec || 0);

  set('statLessons', `${completed} บท`);
  set('statWatch',   `${watchMin} นาที (${watchSec}s)`);
  set('statScore',   `${(summary.average_score || 0).toFixed(1)}%`);

  // Update floating "วุฒิบัตรของฉัน" badge count
  const certCount = allLearnerLessons.filter(l => (l.progress && l.progress.is_completed) || l.certificate).length;
  const floatBadge = document.getElementById('floatingCertCountBadge');
  if (floatBadge) {
    floatBadge.innerText = `${certCount}`;
  }

  renderTxList(summary.recent_records || []);
  renderLearnerLessonsUI();
}

const RECOMMENDED_LESSONS_LIMIT = 5;
let showAllOpenLessons = false;

function toggleShowAllOpenLessons() {
  showAllOpenLessons = !showAllOpenLessons;
  const input = document.getElementById('learnerSearchInput');
  renderLearnerLessonsUI(input ? input.value : '');
}

function renderLearnerLessonsUI(filterText = '') {
  const assignedList   = document.getElementById('assignedLessonsList');
  const openList       = document.getElementById('openLessonsList');
  const assignedBadge  = document.getElementById('assignedBadgeCount');
  const subtitleBadge  = document.getElementById('openLessonsSubtitleBadge');
  const countBadge     = document.getElementById('openLessonsCountBadge');
  const toggleWrap     = document.getElementById('openLessonsToggleWrap');
  const toggleBtn      = document.getElementById('btnToggleAllOpenLessons');

  const query = filterText.toLowerCase().trim();

  // บทเรียนที่ได้รับมอบหมายจากผู้จัดการ: แสดงเฉพาะบทที่ยังเรียนไม่เสร็จ (ทำเสร็จแล้วหายไปอัตโนมัติ)
  const pendingAssigned = allLearnerLessons.filter(l => l.is_assigned && !(l.progress && l.progress.is_completed));
  const completedAssignedCount = allLearnerLessons.filter(l => l.is_assigned && l.progress && l.progress.is_completed).length;
  const open = allLearnerLessons.filter(l => !l.is_hidden);

  if (assignedBadge) {
    assignedBadge.innerText = `${pendingAssigned.length} บทเรียน`;
  }

  // 1. Render Pending Assigned Lessons
  const filteredAssigned = query
    ? pendingAssigned.filter(l => (l.title||'').toLowerCase().includes(query) || (l.summary||'').toLowerCase().includes(query))
    : pendingAssigned;

  if (filteredAssigned.length === 0) {
    if (query) {
      assignedList.innerHTML = `<p style="color:var(--muted); padding:1rem; text-align:center;">ไม่พบบทเรียนที่ได้รับมอบหมายตรงกับคำค้นหา</p>`;
    } else if (completedAssignedCount > 0) {
      assignedList.innerHTML = `<p style="color:var(--success); font-weight:500; padding:1rem; text-align:center;">🎉 เรียนครบทุกบทเรียนที่ได้รับมอบหมายแล้ว (${completedAssignedCount} บทเรียน)</p>`;
    } else {
      assignedList.innerHTML = `<p style="color:var(--muted); padding:1rem; text-align:center;">ไม่มีบทเรียนที่ได้รับมอบหมายในขณะนี้</p>`;
    }
  } else {
    assignedList.innerHTML = filteredAssigned.map(l => createLearnerLessonCard(l, true)).join('');
  }

  // 2. Render All Open Lessons (Default: 5 Recommended Lessons unless searching or expanded)
  const filteredOpen = query ? open.filter(l => (l.title||'').toLowerCase().includes(query) || (l.summary||'').toLowerCase().includes(query)) : open;
  const displayedOpen = (!query && !showAllOpenLessons)
    ? filteredOpen.slice(0, RECOMMENDED_LESSONS_LIMIT)
    : filteredOpen;

  if (subtitleBadge) {
    if (query) {
      subtitleBadge.innerText = `(ผลการค้นหา ${filteredOpen.length} บทเรียน)`;
    } else if (showAllOpenLessons || filteredOpen.length <= RECOMMENDED_LESSONS_LIMIT) {
      subtitleBadge.innerText = `(${displayedOpen.length} บทเรียนแนะนำ)`;
    } else {
      subtitleBadge.innerText = `(แสดง ${RECOMMENDED_LESSONS_LIMIT} บทเรียนแนะนำ จาก ${filteredOpen.length} บทเรียน)`;
    }
  }

  if (countBadge) {
    countBadge.innerText = query
      ? `🔍 พบ ${filteredOpen.length} บทเรียน`
      : `📖 แนะนำ ${displayedOpen.length} / ${filteredOpen.length} บทเรียน`;
  }

  if (toggleWrap && toggleBtn) {
    if (!query && filteredOpen.length > RECOMMENDED_LESSONS_LIMIT) {
      toggleWrap.classList.remove('hidden');
      toggleBtn.innerText = showAllOpenLessons
        ? `🔼 ย่อเหลือ ${RECOMMENDED_LESSONS_LIMIT} บทเรียนแนะนำ`
        : `🔽 ดูทั้งหมด (${filteredOpen.length} บทเรียน)`;
    } else {
      toggleWrap.classList.add('hidden');
    }
  }

  if (displayedOpen.length === 0) {
    openList.innerHTML = `<p style="color:var(--muted); padding:1rem; text-align:center;">ไม่พบบทเรียนที่ตรงกับคำค้นหา</p>`;
  } else {
    openList.innerHTML = displayedOpen.map(l => createLearnerLessonCard(l, false)).join('');
  }
}

function filterLearnerLessons() {
  const input = document.getElementById('learnerSearchInput');
  if (input) {
    renderLearnerLessonsUI(input.value);
  }
}

function createLearnerLessonCard(l, isAssignedSection) {
  const p = l.progress || {};
  const c = l.certificate || null;
  const thumb = l.thumbnail_url || 'https://images.unsplash.com/photo-1622979135225-d2ba269bc1df?q=80&w=400';
  const studyHours = l.study_hours || (c && c.study_hours) || 1;

  let badges = '';
  if (p.is_completed) {
    const tokenTag = c && c.token_id ? ` #${escapeHtml(c.token_id)}` : '';
    badges = `
      <span class="badge badge-ok">✅ สำเร็จแล้ว</span>
      <span class="badge badge-pend">🏅 วุฒิบัตร${tokenTag}</span>
    `;
  } else {
    const preOk  = p.pre_test_passed;
    const vidOk  = p.video_watched;
    const postOk = p.post_test_passed;
    badges = `
      <span class="badge ${preOk ? 'badge-ok' : 'badge-pend'}">ก่อนเรียน: ${preOk ? '✅' : 'รอทำ'}</span>
      <span class="badge ${vidOk ? 'badge-ok' : 'badge-pend'}">วิดีโอ: ${vidOk ? '✅ ครบ' : `${Math.round(p.last_watched_sec||0)}s / ${l.duration_sec}s`}</span>
      <span class="badge ${postOk ? 'badge-ok' : (vidOk ? 'badge-pend' : 'badge-lock')}">หลังเรียน: ${postOk ? '✅' : (vidOk ? 'พร้อมสอบ' : '🔒')}</span>`;
  }

  const assignedTag = l.is_assigned ? `<span class="badge" style="background:rgba(14,165,233,0.14); color:var(--primary); border:1px solid rgba(14,165,233,0.35)">📌 มอบหมาย</span>` : '';
  const hoursTag    = `<span class="badge" style="background:rgba(2,132,199,0.12); color:var(--primary); border:1px solid rgba(2,132,199,0.3)">🕒 ${studyHours} ชั่วโมง</span>`;
  const periodTag   = l.open_period ? `<span class="badge badge-pend">⏰ เปิดให้เรียนถึงวันที่: ${escapeHtml(l.open_period)}</span>` : '';

  const actionBtn = p.is_completed
    ? `<div class="completed-action-group">
         <button class="btn btn-review" style="width:100%;" onclick="openLesson(${l.id})">📖 ทบทวน</button>
         <div class="cert-mini-actions">
           <button class="btn-cert-action print" onclick="printLessonCertificate(${l.id})" title="พิมพ์วุฒิบัตร">🖨️ พิมพ์</button>
           <button class="btn-cert-action email" onclick="emailLessonCertificate(${l.id})" title="ส่งวุฒิบัตรเข้าอีเมล">📧 ส่งเมล</button>
         </div>
       </div>`
    : `<button class="btn btn-lesson" onclick="openLesson(${l.id})">เข้าเรียน ›</button>`;

  return `
    <div class="lesson-item">
      <div class="lesson-thumb-wrap">
        <img class="lesson-thumb" src="${thumb}" alt="${escapeHtml(l.title)}" loading="lazy">
        <div class="lesson-order">${l.sort_order}</div>
      </div>
      <div class="lesson-info">
        <div class="lesson-name">${escapeHtml(l.title)}</div>
        <div class="lesson-desc" style="line-height:1.5;">${escapeHtml(l.summary || l.description || '')}</div>
        <div class="lesson-badges">
          ${assignedTag}
          ${hoursTag}
          ${periodTag}
          ${badges}
        </div>
      </div>
      <div class="lesson-action">
        ${actionBtn}
      </div>
    </div>
  `;
}

async function printLessonCertificate(lessonId) {
  const l = allLearnerLessons.find(x => x.id === lessonId);
  if (!l) return;
  await loadSignatoriesCache();
  const cert = l.certificate || {};
  const tokenId     = cert.token_id || `SBT-${l.course_id || 1}-${l.id}-${currentUserID}`;
  const title       = l.title || cert.lesson_title || 'บทเรียน';
  const recipient   = currentUser ? (currentUser.name || currentUser.email || currentUser.wallet_address) : 'ผู้เรียน';
  const txHash      = cert.transaction_hash || (l.progress ? l.progress.on_chain_tx_hash : '') || '';
  const issuedDate  = cert.issued_at ? new Date(cert.issued_at).toLocaleDateString('th-TH', { year: 'numeric', month: 'long', day: 'numeric' }) : new Date().toLocaleDateString('th-TH', { year: 'numeric', month: 'long', day: 'numeric' });
  const studyHours  = l.study_hours || cert.study_hours || 1;
  const durationSec = l.duration_sec || cert.lesson_duration_sec || 60;
  const signatories = resolveSignatoriesByIDs(l.signatory_ids);

  printCertificate(tokenId, title, recipient, txHash, issuedDate, studyHours, durationSec, signatories);
}

async function emailLessonCertificate(lessonId) {
  const res = await fetchJSON(`/api/lessons/${lessonId}/certificate/email`, 'POST');
  if (res && res.success) {
    alert('🎉 ' + res.message);
  } else {
    alert('❌ ไม่สามารถส่งวุฒิบัตรเข้าอีเมลได้: ' + (res ? res.message : 'server error'));
  }
}

// ─── Modal วุฒิบัตรของฉัน ───
let myCertsCache = [];
let myCertsTab = 'active';

async function openMyCertificatesModal() {
  const modal = document.getElementById('myCertificatesModal');
  const container = document.getElementById('myCertificatesList');
  container.innerHTML = '<p style="color:var(--muted); padding:2rem; text-align:center;">⏳ กำลังโหลดวุฒิบัตรของคุณ...</p>';
  modal.classList.remove('hidden');

  const res = await fetchJSON('/api/certificates/my');
  myCertsCache = (res && res.success && Array.isArray(res.certificates)) ? res.certificates : [];
  myCertsTab = 'active';
  renderMyCertificatesModal();
}

function switchMyCertificatesTab(tab) {
  myCertsTab = tab === 'old' ? 'old' : 'active';
  renderMyCertificatesModal();
}

function renderMyCertificatesModal() {
  const container = document.getElementById('myCertificatesList');
  const activeCerts = myCertsCache.filter(c => c.lesson_exists !== false);
  const oldCerts = myCertsCache.filter(c => c.lesson_exists === false);

  const activeCountEl = document.getElementById('certActiveCount');
  const oldCountEl = document.getElementById('certOldCount');
  if (activeCountEl) activeCountEl.innerText = activeCerts.length;
  if (oldCountEl) oldCountEl.innerText = oldCerts.length;

  const btnActive = document.getElementById('btnCertTabActive');
  const btnOld = document.getElementById('btnCertTabOld');
  if (btnActive) btnActive.className = `btn btn-sm ${myCertsTab === 'active' ? 'btn-primary' : 'btn-wallet'}`;
  if (btnOld) btnOld.className = `btn btn-sm ${myCertsTab === 'old' ? 'btn-primary' : 'btn-wallet'}`;

  const list = myCertsTab === 'old' ? oldCerts : activeCerts;
  if (list.length > 0) {
    container.innerHTML = list.map(cert => {
      const lessonTitle = cert.lesson_title || (cert.course ? cert.course.title : 'บทเรียน');
      const courseTitle = cert.course ? cert.course.title : 'หลักสูตรการพยาบาล';
      const issuedDate  = cert.issued_at ? new Date(cert.issued_at).toLocaleDateString('th-TH', { year: 'numeric', month: 'long', day: 'numeric' }) : '-';
      const txShort     = cert.transaction_hash ? `${cert.transaction_hash.slice(0, 8)}...${cert.transaction_hash.slice(-6)}` : 'บันทึกแล้ว';
      const txHref      = cert.transaction_hash ? `https://sepolia.etherscan.io/tx/${cert.transaction_hash}` : '#';
      const recipient   = cert.user ? (cert.user.name || cert.user.email || cert.user.wallet_address) : (currentUser ? currentUser.name : '');
      const matchedLesson = allLearnerLessons.find(x => x.id === cert.lesson_id);
      const studyHours  = cert.study_hours || (matchedLesson ? matchedLesson.study_hours : 0) || 1;
      const durationSec = cert.lesson_duration_sec || (matchedLesson ? matchedLesson.duration_sec : 0) || 60;
      const isOld       = cert.lesson_exists === false;

      return `
        <div class="cert-item-card">
          <div style="display:flex; justify-content:space-between; align-items:center; gap:0.5rem; margin-bottom:0.35rem;">
            <div style="font-size:0.74rem; color:#d97706; font-weight:700;">🏅 วุฒิบัตรอิเล็กทรอนิกส์รับรองผลการศึกษา</div>
            ${isOld ? '<span style="font-size:0.7rem; padding:0.15rem 0.45rem; border-radius:999px; background:rgba(148,163,184,0.2); color:var(--muted);">บทเรียนถูกลบแล้ว</span>' : ''}
          </div>
          <h3 style="font-size:1.08rem; color:var(--text); margin-bottom:0.3rem; line-height:1.45;">${escapeHtml(lessonTitle)}</h3>
          <div style="font-size:0.8rem; color:var(--primary); font-weight:600; margin-bottom:0.55rem; line-height:1.5;">
            <div>🕒 เวลาเรียน: ${studyHours} ชั่วโมง</div>
            <div>📚 หลักสูตร: ${escapeHtml(courseTitle)}</div>
          </div>
          <div style="font-size:0.81rem; color:var(--muted); margin-bottom:0.85rem; line-height:1.65;">
            <div>ผู้รับ: <strong style="color:var(--text);">${escapeHtml(recipient)}</strong></div>
            <div style="margin:0.3rem 0; padding:0.4rem 0.65rem; background:var(--bg); border-left:3px solid var(--primary); border-radius:4px;">
              <div style="font-weight:600; color:var(--primary);">ออกโดย:</div>
              <div style="color:var(--text); font-weight:500; line-height:1.5;">${ISSUING_ORG_MULTILINE_HTML}</div>
            </div>
            <div>รหัสวุฒิบัตร: <strong>#${escapeHtml(cert.token_id)}</strong> | วันที่: ${issuedDate}</div>
            <div>คะแนนเฉลี่ย: <strong>${(cert.average_score||100).toFixed(1)}%</strong> | ธุรกรรม: <a href="${txHref}" target="_blank" rel="noopener" style="color:var(--primary)">${txShort}</a></div>
          </div>
          <div style="display:flex; gap:0.5rem; flex-wrap:wrap;">
            <button class="btn btn-sm btn-cert-action print" onclick="printCertificate('${escapeHtml(cert.token_id)}', '${escapeHtml(lessonTitle)}', '${escapeHtml(recipient)}', '${escapeHtml(cert.transaction_hash)}', '${escapeHtml(issuedDate)}', ${studyHours}, ${durationSec})">
              🖨️ พิมพ์
            </button>
            <button class="btn btn-sm btn-primary" onclick="emailCertificate(${cert.id})">
              📧 ส่งเมล
            </button>
            ${isOld ? `<button class="btn btn-sm" style="background:rgba(239,68,68,0.12); color:#ef4444; border:1px solid rgba(239,68,68,0.35);" onclick="deleteMyCertificate(${cert.id})">🗑️ ลบ</button>` : ''}
          </div>
        </div>
      `;
    }).join('');
  } else if (myCertsTab === 'old') {
    container.innerHTML = `
      <div style="text-align:center; padding:3rem 1rem; color:var(--muted);">
        <div style="font-size:3rem; margin-bottom:0.5rem">🗂️</div>
        <h4>ไม่มีวุฒิบัตรเก่าจากบทเรียนที่ถูกลบ</h4>
      </div>
    `;
  } else {
    container.innerHTML = `
      <div style="text-align:center; padding:3rem 1rem; color:var(--muted);">
        <div style="font-size:3rem; margin-bottom:0.5rem">🏅</div>
        <h4>คุณยังไม่มีวุฒิบัตรในขณะนี้</h4>
        <p style="font-size:0.85rem; line-height:1.6;">เรียนบทเรียนให้ครบตามเวลาและสอบผ่านเกณฑ์<br>เพื่อรับวุฒิบัตรอิเล็กทรอนิกส์</p>
      </div>
    `;
  }
}

async function deleteMyCertificate(certId) {
  if (!confirm('ต้องการลบวุฒิบัตรเก่านี้ใช่หรือไม่? (เมื่อลบแล้วจะไม่แสดงในรายการอีก)')) return;
  const res = await fetchJSON(`/api/certificates/${certId}`, 'DELETE');
  if (res && res.success) {
    myCertsCache = myCertsCache.filter(c => c.id !== certId);
    renderMyCertificatesModal();
  } else {
    alert('❌ ' + ((res && res.message) || 'ไม่สามารถลบวุฒิบัตรได้'));
  }
}

function closeMyCertificatesModal() {
  document.getElementById('myCertificatesModal').classList.add('hidden');
}

function openPrintPreviewModal(title, htmlContent) {
  const titleEl = document.getElementById('printPreviewTitle');
  const previewContent = document.getElementById('printPreviewContent');
  const printable = document.getElementById('printableArea');
  const modal = document.getElementById('printPreviewModal');

  if (titleEl) titleEl.innerText = title || 'ตัวอย่างเอกสารก่อนพิมพ์';
  if (previewContent) previewContent.innerHTML = htmlContent;
  if (printable) printable.innerHTML = htmlContent;
  if (modal) modal.classList.remove('hidden');
}

function closePrintPreviewModal() {
  const modal = document.getElementById('printPreviewModal');
  if (modal) modal.classList.add('hidden');
}

function triggerNativePrint() {
  const printable = document.getElementById('printableArea');
  const htmlContent = printable ? printable.innerHTML : '';
  if (!htmlContent) return;

  // Try opening a clean print window for 100% isolated, high-resolution printing
  const printWin = window.open('', '_blank', 'width=960,height=780');
  if (printWin && printWin.document) {
    printWin.document.open();
    printWin.document.write(`<!DOCTYPE html>
<html lang="th">
<head>
  <meta charset="UTF-8">
  <title>เอกสารสำหรับพิมพ์</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Prompt:wght@300;400;500;600;700&display=swap" rel="stylesheet">
  <style>
    @page { size: A4; margin: 10mm; }
    * { box-sizing: border-box; -webkit-print-color-adjust: exact !important; print-color-adjust: exact !important; }
    body { margin: 0; padding: 16px; font-family: 'Prompt', sans-serif; background: #ffffff; color: #0f172a; }
    @media print { body { padding: 0; } .no-print { display: none !important; } }
  </style>
</head>
<body>
  ${htmlContent}
  <script>
    window.onload = function() {
      setTimeout(function() {
        window.focus();
        window.print();
      }, 350);
    };
  <\/script>
</body>
</html>`);
    printWin.document.close();
    return;
  }

  // Fallback to in-page @media print
  if (printable) printable.classList.remove('hidden');
  setTimeout(() => {
    window.print();
    if (printable) printable.classList.add('hidden');
  }, 150);
}

function printCertificate(tokenId, courseTitle, recipient, txHash, issuedDate, studyHours = 1, durationSec = 60, signatories = []) {
  const walletAddr = currentUser && currentUser.wallet_address ? currentUser.wallet_address : '0xVerifiedSoulboundLearner';
  const walletShort = walletAddr.length > 20 ? `${walletAddr.slice(0, 10)}...${walletAddr.slice(-8)}` : walletAddr;
  const txDisplay = txHash && txHash.length > 22 ? `${txHash.slice(0, 14)}...${txHash.slice(-10)}` : (txHash || 'บันทึกบนบล็อกเชนแล้ว');
  const hoursVal = (studyHours && Number(studyHours) > 0) ? Number(studyHours) : 1;

  const certHTML = `
    <div style="max-width:840px; margin:0 auto; background:#ffffff; color:#0f172a; font-family:'Prompt',sans-serif; border-radius:18px; overflow:hidden; border:3px solid #38bdf8; box-shadow:0 12px 32px rgba(2,132,199,0.16);">
      <!-- Top White-Sky Blue Nursing Header -->
      <div style="background:linear-gradient(135deg, #0284c7 0%, #0369a1 52%, #1e3a8a 100%); color:#ffffff; padding:26px 34px 22px; text-align:center; position:relative; border-bottom:4px solid #eab308;">
        <div style="display:inline-flex; align-items:center; gap:8px; background:rgba(255,255,255,0.16); border:1px solid rgba(255,255,255,0.45); color:#fef08a; padding:4px 16px; border-radius:999px; font-size:11px; font-weight:700; letter-spacing:1px; margin-bottom:8px;">
          <span>🩺 วุฒิบัตรอิเล็กทรอนิกส์ทางการพยาบาล</span>
        </div>
        <h1 style="margin:2px 0 6px; font-size:28px; font-weight:700; color:#ffffff; letter-spacing:0.4px;">ใบประกาศนียบัตรรับรองผลการศึกษา</h1>
        <div style="font-size:13.5px; color:#e0f2fe; font-weight:600; margin-top:4px; line-height:1.55;">
          ${ISSUING_ORG_MULTILINE_HTML}
        </div>
      </div>

      <!-- Inner Ornamental Body -->
      <div style="padding:32px 42px 28px; text-align:center; position:relative; background:radial-gradient(circle at top right, rgba(186,230,253,0.35), transparent 48%), radial-gradient(circle at bottom left, rgba(254,240,138,0.22), transparent 48%);">
        <p style="font-size:14.5px; color:#475569; margin:0 0 8px;">ขอมอบใบประกาศนียบัตรดิจิทัลฉบับนี้เพื่อแสดงว่า</p>

        <!-- Recipient Name -->
        <div style="margin:4px auto 16px; display:inline-block; padding:4px 36px 10px; border-bottom:2.5px solid #0284c7;">
          <div style="font-size:27px; font-weight:700; color:#0c4a6e; letter-spacing:0.3px;">${escapeHtml(recipient || 'ผู้เรียน')}</div>
          <div style="font-size:11px; color:#64748b; font-family:monospace; margin-top:2px;">กระเป๋าบล็อกเชน: ${escapeHtml(walletShort)}</div>
        </div>

        <p style="font-size:14.5px; color:#334155; margin:0 0 14px;">ได้ผ่านการเรียนรู้ ทดสอบวัดผล และบรรลุเกณฑ์มาตรฐานการประเมินในบทเรียน</p>

        <!-- Completed Lesson Box + Study Hours -->
        <div style="background:linear-gradient(135deg, #f0f9ff 0%, #e0f2fe 100%); border:1.5px solid #7dd3fc; border-left:5px solid #0284c7; border-radius:14px; padding:16px 24px; max-width:680px; margin:0 auto 18px;">
          <div style="font-size:11.5px; color:#0369a1; font-weight:700; margin-bottom:4px;">บทเรียนที่สำเร็จการศึกษา</div>
          <div style="font-size:20px; font-weight:700; color:#0c4a6e; line-height:1.45; margin-bottom:8px;">“${escapeHtml(courseTitle)}”</div>
          <div style="display:inline-flex; align-items:center; gap:8px; background:#ffffff; border:1px solid #38bdf8; color:#0369a1; padding:5px 18px; border-radius:999px; font-size:14px; font-weight:700; box-shadow:0 2px 6px rgba(2,132,199,0.1);">
            🕒 จำนวนเวลาเรียน: ${hoursVal} ชั่วโมง
          </div>
        </div>

        <!-- Issuing Organization Banner (Multiline) -->
        <div style="margin:0 auto 20px; max-width:680px; background:#f8fafc; border:1px solid #bae6fd; border-radius:10px; padding:10px 18px; font-size:13px; color:#1e3a5f; line-height:1.6;">
          <div style="color:#0284c7; font-weight:700; margin-bottom:2px;">ออกโดย:</div>
          <div style="font-weight:700;">${ISSUING_ORG_MULTILINE_HTML}</div>
        </div>

        <!-- Achievement Pills -->
        <div style="display:flex; justify-content:center; gap:12px; flex-wrap:wrap; margin-bottom:24px;">
          <span style="background:#ecfdf5; border:1px solid #6ee7b7; color:#047857; padding:5px 14px; border-radius:999px; font-size:12px; font-weight:600;">✅ ผ่านเกณฑ์การประเมิน (70%+)</span>
          <span style="background:#e0f2fe; border:1px solid #7dd3fc; color:#0369a1; padding:5px 14px; border-radius:999px; font-size:12px; font-weight:600;">⏱️ เวลาเรียนครบ ${hoursVal} ชั่วโมง</span>
          <span style="background:#fefce8; border:1px solid #fde047; color:#a16207; padding:5px 14px; border-radius:999px; font-size:12px; font-weight:600;">🏅 บันทึกบนบล็อกเชน</span>
        </div>

        <!-- 3-Column Footer: On-Chain Proof / Seal / Issuing Organization Signature -->
        <div style="display:grid; grid-template-columns:1.2fr 0.8fr 1.3fr; align-items:center; gap:14px; border-top:1.5px dashed #bae6fd; padding-top:20px; text-align:left;">
          <!-- Left: On-Chain Metadata -->
          <div style="background:#f0f9ff; border:1px solid #bae6fd; border-radius:10px; padding:10px 14px; font-size:11.5px; color:#334155; line-height:1.65;">
            <div><strong style="color:#0c4a6e;">หมายเลขวุฒิบัตร:</strong> <span style="color:#0284c7; font-weight:700;">#${escapeHtml(tokenId)}</span></div>
            <div><strong style="color:#0c4a6e;">จำนวนเวลาเรียน:</strong> <strong>${hoursVal} ชั่วโมง</strong></div>
            <div><strong style="color:#0c4a6e;">วันที่ออกใบประกาศ:</strong> ${escapeHtml(issuedDate)}</div>
            <div><strong style="color:#0c4a6e;">รหัสธุรกรรม:</strong> <span style="font-family:monospace; color:#0284c7;">${escapeHtml(txDisplay)}</span></div>
          </div>

          <!-- Center: Gold & Sky Seal -->
          <div style="text-align:center;">
            <div style="width:82px; height:82px; margin:0 auto; border-radius:50%; background:linear-gradient(135deg, #fef08a 0%, #eab308 55%, #ca8a04 100%); border:3px double #0284c7; display:flex; flex-direction:column; align-items:center; justify-content:center; color:#451a03; box-shadow:0 4px 12px rgba(2,132,199,0.22);">
              <div style="font-size:22px; line-height:1;">🩺</div>
              <div style="font-size:8.5px; font-weight:800; margin-top:2px;">รับรองผล</div>
              <div style="font-size:7.5px; font-weight:700;">${hoursVal} ชั่วโมง</div>
            </div>
          </div>

          <!-- Right: Signatories (or Issuing Nursing Department Fallback) -->
          ${renderCertificateSignatoriesHTML(signatories)}
        </div>
      </div>
    </div>
  `;

  openPrintPreviewModal(`ใบประกาศนียบัตร: ${courseTitle}`, certHTML);
}

async function emailCertificate(certId) {
  const res = await fetchJSON(`/api/certificates/${certId}/email`, 'POST');
  if (res && res.success) {
    alert('🎉 ' + res.message);
  } else {
    alert('❌ ไม่สามารถส่งอีเมลได้: ' + (res ? res.message : 'server error'));
  }
}

// ============================================================
//  ROLE 2: 👨‍🏫 ผู้สร้างบทเรียน (CREATOR STUDIO)
// ============================================================
async function loadCreatorDashboard() {
  const data = await fetchJSON(`/api/courses/creator/${currentUserID}`);
  if (!data || !data.success || !data.course) return;

  currentCourse = data.course;
  currentCourseID = currentCourse.id;
  allCreatorLessons = currentCourse.lessons || [];

  // Update Creator Metrics
  const total = allCreatorLessons.length;
  const active = allCreatorLessons.filter(l => !l.is_hidden).length;
  const hidden = allCreatorLessons.filter(l => l.is_hidden).length;

  let totalSec = 0;
  allCreatorLessons.forEach(l => { totalSec += (l.duration_sec || 0); });
  const totalMin = Math.round(totalSec / 60);

  set('creatorTotalLessons', total);
  set('creatorActiveLessons', active);
  set('creatorHiddenLessons', hidden);
  set('creatorTotalDuration', `${totalMin} นาที`);

  renderCreatorLessonsUI();
}

function renderCreatorLessonsUI() {
  const list = document.getElementById('creatorLessonsList');
  if (!allCreatorLessons || allCreatorLessons.length === 0) {
    list.innerHTML = `
      <div style="background:var(--card); border:1px dashed var(--border); border-radius:var(--r); padding:3rem; text-align:center; color:var(--muted)">
        <p style="font-size:1.15rem; margin-bottom:0.75rem; color:var(--text)">📚 คุณยังไม่มีบทเรียนในหลักสูตรนี้</p>
        <p style="font-size:0.85rem; margin-bottom:1.25rem">เริ่มต้นสร้างบทเรียนแรกของคุณ พร้อมแนบวิดีโอ YouTube และแบบทดสอบ</p>
        <button class="btn btn-primary" onclick="openCreateLessonModal()">➕ สร้างบทเรียนแรกของคุณ</button>
      </div>`;
    return;
  }

  list.innerHTML = allCreatorLessons.map(l => {
    const thumb = l.thumbnail_url || 'https://images.unsplash.com/photo-1622979135225-d2ba269bc1df?q=80&w=400';
    const isHidden = l.is_hidden;
    const studyHours = l.study_hours || 1;

    const statusBadge = isHidden
      ? `<span class="badge" style="background:rgba(239,68,68,0.15); color:var(--danger); border-color:rgba(239,68,68,0.3)">🙈 ซ่อนอยู่</span>`
      : `<span class="badge badge-ok">👁️ กำลังแสดง</span>`;

    const openPeriod = l.open_period || 'เปิดตลอดเวลา';
    const rs = l.rating_stats || { average_rating: 0, total_ratings: 0 };

    return `
      <div class="lesson-item" onclick="openLessonRatingsModal(${l.id}, '${escapeHtml(l.title)}')" style="cursor:pointer; ${isHidden ? 'opacity:0.75; border-color:rgba(239,68,68,0.3);' : ''}" title="คลิกที่การ์ดเพื่อดูสถิติคะแนน 1-5 ดาว">
        <div class="lesson-thumb-wrap">
          <img class="lesson-thumb" src="${thumb}" alt="${escapeHtml(l.title)}" loading="lazy">
          <div class="lesson-order">${l.sort_order}</div>
        </div>
        <div class="lesson-info">
          <div class="lesson-name">${escapeHtml(l.title)}</div>
          <div class="lesson-desc">${escapeHtml(l.summary || l.description || '')}</div>
          <div class="lesson-badges">
            ${statusBadge}
            <span class="badge" style="background:rgba(250,204,21,0.14); color:#b45309; border:1px solid rgba(250,204,21,0.45)">⭐ ${(rs.average_rating || 0).toFixed(1)}/5 (${rs.total_ratings || 0} รีวิว) • 👆 กดดูสถิติ</span>
            <span class="badge" style="background:rgba(2,132,199,0.12); color:var(--primary); border:1px solid rgba(2,132,199,0.3)">🕒 เวลาเรียน: ${studyHours} ชั่วโมง</span>
            <span class="badge badge-ok">⏱️ ขั้นต่ำ: ${l.duration_sec}s</span>
            <span class="badge badge-pend">⏰ เปิดให้เรียนถึงวันที่: ${escapeHtml(openPeriod)}</span>
          </div>
        </div>
        <div class="lesson-action" onclick="event.stopPropagation()" style="min-width:150px; display:flex; flex-direction:column; gap:0.35rem;">
          <button class="btn btn-sm btn-wallet" onclick="openEditLessonModal(${l.id})">✏️ แก้ไข</button>
          <button class="btn btn-sm btn-wallet" onclick="openScheduleModal(${l.id}, '${escapeHtml(openPeriod)}')">⏰ ตั้งเวลา</button>
          <button class="btn btn-sm btn-wallet" onclick="toggleHideLesson(${l.id})">${isHidden ? '👁️ แสดง' : '🙈 ซ่อน'}</button>
          <button class="btn btn-sm btn-review" onclick="openLesson(${l.id})">👁️ ทดสอบ</button>
          <button class="btn btn-sm" style="background:rgba(239,68,68,0.12); color:var(--danger); border:1px solid rgba(239,68,68,0.3)" onclick="deleteLesson(${l.id}, '${escapeHtml(l.title)}')">🗑️ ลบ</button>
        </div>
      </div>
    `;
  }).join('');
}

// ─── Creator Actions ───
async function toggleHideLesson(lessonId) {
  const res = await fetchJSON(`/api/lessons/${lessonId}/toggle-hide`, 'POST');
  if (res && res.success) {
    await loadCreatorDashboard();
  } else {
    alert('❌ ไม่สามารถเปลี่ยนสถานะได้: ' + (res ? res.message : 'server error'));
  }
}

const THAI_MONTHS = [
  'มกราคม', 'กุมภาพันธ์', 'มีนาคม', 'เมษายน', 'พฤษภาคม', 'มิถุนายน',
  'กรกฎาคม', 'สิงหาคม', 'กันยายน', 'ตุลาคม', 'พฤศจิกายน', 'ธันวาคม'
];

function applyDateTimeToField(dateInputId, timeInputId, targetInputId) {
  const dateEl = document.getElementById(dateInputId);
  const timeEl = document.getElementById(timeInputId);
  const target = document.getElementById(targetInputId);
  if (!dateEl || !target || !dateEl.value) return;

  const parts = dateEl.value.split('-'); // YYYY-MM-DD
  if (parts.length !== 3) return;
  const yearBE = parseInt(parts[0], 10) + 543;
  const monthIdx = parseInt(parts[1], 10) - 1;
  const day = parseInt(parts[2], 10);

  let timeStr = (timeEl && timeEl.value) ? timeEl.value.replace(':', '.') : '10.00';
  target.value = `${day} ${THAI_MONTHS[monthIdx] || ''} ${yearBE} ${timeStr} น.`;
}

function openScheduleModal(lessonId, currentPeriod) {
  document.getElementById('scheduleLessonId').value = lessonId;
  document.getElementById('inputSchedulePeriod').value = currentPeriod || 'เปิดตลอดเวลา';
  document.getElementById('scheduleModal').classList.remove('hidden');
}

function closeScheduleModal() {
  document.getElementById('scheduleModal').classList.add('hidden');
}

async function submitSchedule(e) {
  e.preventDefault();
  const id = document.getElementById('scheduleLessonId').value;
  const period = document.getElementById('inputSchedulePeriod').value.trim();

  const res = await fetchJSON(`/api/lessons/${id}/schedule`, 'POST', { open_period: period });
  if (res && res.success) {
    alert('✅ อัปเดตระยะเวลาเปิดให้เรียนเรียบร้อยแล้ว');
    closeScheduleModal();
    await loadCreatorDashboard();
  } else {
    alert('❌ ไม่สามารถบันทึกเวลาได้: ' + (res ? res.message : 'server error'));
  }
}

function openEditLessonModal(lessonId) {
  const l = allCreatorLessons.find(x => x.id === lessonId);
  if (!l) return;

  document.getElementById('editLessonId').value = l.id;
  document.getElementById('editLessonTitle').value = l.title || '';
  document.getElementById('editLessonSummary').value = l.summary || l.description || '';
  document.getElementById('editLessonYoutube').value = l.youtube_url || '';
  document.getElementById('editLessonDuration').value = l.duration_sec || 60;
  const hoursInput = document.getElementById('editLessonStudyHours');
  if (hoursInput) hoursInput.value = l.study_hours || 1;
  document.getElementById('editLessonThumbnail').value = l.thumbnail_url || '';
  document.getElementById('editLessonDocument').value = l.document_url || '';
  document.getElementById('editLessonOpenPeriod').value = l.open_period || 'เปิดตลอดเวลา';
  document.getElementById('editLessonIsHidden').value = l.is_hidden ? 'true' : 'false';
  const passEl = document.getElementById('editLessonPassScore');
  if (passEl) passEl.value = l.pass_score_pct || 70;
  const noPreEl = document.getElementById('editLessonDisablePreTest');
  if (noPreEl) {
    const hasPre = !l.disable_pre_test && (l.quizzes || []).some(q => q.type === 'lesson_pre_test');
    noPreEl.checked = !hasPre;
  }
  const noPostEl = document.getElementById('editLessonDisablePostTest');
  if (noPostEl) {
    const hasPost = !l.disable_post_test && (l.quizzes || []).some(q => q.type === 'lesson_post_test');
    noPostEl.checked = !hasPost;
  }

  renderLessonSignatoryCheckboxes('editLessonSignatoriesBox', 'editSigCheck', l.signatory_ids);
  document.getElementById('editLessonModal').classList.remove('hidden');
}

function closeEditLessonModal() {
  document.getElementById('editLessonModal').classList.add('hidden');
}

async function uploadLessonCover(fileInputId, targetUrlInputId, statusId) {
  const fileEl = document.getElementById(fileInputId);
  const urlEl = document.getElementById(targetUrlInputId);
  const statusEl = document.getElementById(statusId);
  if (!fileEl || !fileEl.files || fileEl.files.length === 0) return;

  const file = fileEl.files[0];
  const fd = new FormData();
  fd.append('cover', file);

  if (statusEl) statusEl.innerText = '⏳ กำลังอัปโหลดภาพปก...';
  try {
    const res = await fetch('/api/lessons/cover', {
      method: 'POST',
      headers: {
        'X-User-ID': String(currentUserID || ''),
        'X-Wallet-Address': String((currentUser && currentUser.wallet_address) || '')
      },
      body: fd
    });
    const data = await res.json();
    if (data && data.success && data.url) {
      if (urlEl) urlEl.value = data.url;
      if (statusEl) statusEl.innerText = `✅ อัปโหลดแล้ว: ${data.url}`;
    } else {
      if (statusEl) statusEl.innerText = `❌ ${(data && data.message) || 'อัปโหลดไม่สำเร็จ'}`;
    }
  } catch (err) {
    if (statusEl) statusEl.innerText = '❌ เกิดข้อผิดพลาดในการอัปโหลด';
  } finally {
    fileEl.value = '';
  }
}

async function submitEditLesson(e) {
  e.preventDefault();
  const id = document.getElementById('editLessonId').value;
  const hoursInput = document.getElementById('editLessonStudyHours');
  const payload = {
    title: document.getElementById('editLessonTitle').value.trim(),
    summary: document.getElementById('editLessonSummary').value.trim(),
    description: document.getElementById('editLessonSummary').value.trim(),
    youtube_url: document.getElementById('editLessonYoutube').value.trim(),
    duration_sec: parseFloat(document.getElementById('editLessonDuration').value) || 60,
    study_hours: hoursInput ? (parseFloat(hoursInput.value) || 1) : 1,
    thumbnail_url: document.getElementById('editLessonThumbnail').value.trim(),
    document_url: document.getElementById('editLessonDocument').value.trim(),
    open_period: document.getElementById('editLessonOpenPeriod').value.trim(),
    is_hidden: document.getElementById('editLessonIsHidden').value === 'true',
    pass_score_pct: parseFloat(document.getElementById('editLessonPassScore')?.value) || 70,
    disable_pre_test: !!(document.getElementById('editLessonDisablePreTest')?.checked),
    disable_post_test: !!(document.getElementById('editLessonDisablePostTest')?.checked),
    signatory_ids: collectSelectedSignatoryIDs('editSigCheck')
  };

  const res = await fetchJSON(`/api/lessons/${id}`, 'PUT', payload);
  if (res && res.success) {
    alert('🎉 บันทึกการแก้ไขบทเรียนเรียบร้อยแล้ว!');
    closeEditLessonModal();
    await loadCreatorDashboard();
  } else {
    alert('❌ ไม่สามารถแก้ไขได้: ' + (res ? res.message : 'server error'));
  }
}

async function deleteLesson(lessonId, title) {
  if (!confirm(`คุณต้องการลบบทเรียน "${title}" ใช่หรือไม่?`)) return;
  const res = await fetchJSON(`/api/lessons/${lessonId}`, 'DELETE');
  if (res && res.success) {
    alert('✅ ลบบทเรียนเรียบร้อยแล้ว');
    loadCreatorDashboard();
  } else {
    alert('❌ ไม่สามารถลบได้: ' + (res ? res.message : 'server error'));
  }
}

// ============================================================
//  ROLE 3: 👔 ผู้จัดการ (MANAGER PORTAL)
// ============================================================
async function loadManagerDashboard() {
  const data = await fetchJSON('/api/manager/dashboard');
  if (!data || !data.success) return;

  allManagerLessons = data.lessons || [];
  allSystemLearners = data.learners || [];
  allSystemCreators = data.creators || [];
  allSystemManagers = data.managers || [];

  renderManagerUI();
  renderAddManagerModalUI();
}

function openAddManagerModal() {
  const modal = document.getElementById('addManagerModal');
  if (modal) modal.classList.remove('hidden');
  loadManagerDashboard();
}

function closeAddManagerModal() {
  const modal = document.getElementById('addManagerModal');
  if (modal) modal.classList.add('hidden');
}

function renderAddManagerModalUI() {
  const selectEl = document.getElementById('selectCreatorForManager');
  const candidatesEl = document.getElementById('creatorCandidatesList');
  const managersEl = document.getElementById('currentManagersList');

  if (selectEl) {
    const eligibleCreators = allSystemCreators.filter(c => !c.is_manager);
    if (eligibleCreators.length === 0) {
      selectEl.innerHTML = `<option value="">-- ผู้สร้างบทเรียนทุกคนได้รับสิทธิ์เป็นผู้จัดการครบแล้ว --</option>`;
    } else {
      selectEl.innerHTML = eligibleCreators.map(c => {
        const emailStr = c.email ? ` (${escapeHtml(c.email)})` : '';
        return `<option value="${c.id}">👨‍🏫 ${escapeHtml(c.name)}${emailStr}</option>`;
      }).join('');
    }
  }

  if (candidatesEl) {
    if (!allSystemCreators || allSystemCreators.length === 0) {
      candidatesEl.innerHTML = `<div style="padding:1rem; text-align:center; color:var(--muted); border:1px dashed var(--border); border-radius:var(--r);">ยังไม่มีบัญชีผู้สร้างบทเรียนในระบบ</div>`;
    } else {
      candidatesEl.innerHTML = allSystemCreators.map(c => {
        const emailStr = c.email ? escapeHtml(c.email) : 'ไม่ระบุอีเมล';
        const actionHtml = c.is_manager
          ? `<span class="badge badge-ok" style="padding:0.3rem 0.75rem;">✅ เป็นผู้จัดการแล้ว</span>`
          : `<button type="button" class="btn btn-sm btn-primary" onclick="promoteCreatorById(${c.id}, '${escapeHtml(c.name)}')">👔➕ เพิ่ม</button>`;
        return `
          <div style="display:flex; justify-content:space-between; align-items:center; gap:0.75rem; flex-wrap:wrap; background:var(--card); border:1px solid var(--border); border-radius:var(--r); padding:0.75rem 1rem;">
            <div>
              <div style="font-weight:600; color:var(--text);">👨‍🏫 ${escapeHtml(c.name)}</div>
              <div style="font-size:0.8rem; color:var(--muted);">อีเมล: ${emailStr}</div>
            </div>
            <div>${actionHtml}</div>
          </div>
        `;
      }).join('');
    }
  }

  if (managersEl) {
    if (!allSystemManagers || allSystemManagers.length === 0) {
      managersEl.innerHTML = `<div style="padding:1rem; text-align:center; color:var(--muted);">ยังไม่มีรายชื่อผู้จัดการ</div>`;
    } else {
      const canRemove = allSystemManagers.length > 1;
      managersEl.innerHTML = allSystemManagers.map(m => {
        const emailStr = m.email ? escapeHtml(m.email) : '-';
        const isSelf = currentUser && currentUser.email && m.email && currentUser.email.toLowerCase() === m.email.toLowerCase();
        return `
          <div style="display:flex; justify-content:space-between; align-items:center; gap:0.75rem; flex-wrap:wrap; background:var(--bg); border:1px solid var(--border); border-radius:var(--r); padding:0.75rem 1rem;">
            <div>
              <div style="font-weight:600; color:var(--text);">👔 ${escapeHtml(m.name)} ${isSelf ? '<span class="badge badge-ok">บัญชีของคุณ</span>' : ''}</div>
              <div style="font-size:0.8rem; color:var(--muted);">อีเมล: ${emailStr}</div>
            </div>
            <div>
              ${canRemove ? `<button type="button" class="btn btn-sm" style="background:rgba(225,29,72,0.1); color:var(--danger); border:1px solid rgba(225,29,72,0.3);" onclick="removeManagerById(${m.id}, '${escapeHtml(m.name)}')">🗑️ ถอนสิทธิ์</button>` : `<span style="font-size:0.78rem; color:var(--muted);">ผู้จัดการหลัก</span>`}
            </div>
          </div>
        `;
      }).join('');
    }
  }
}

async function promoteSelectedCreatorToManager() {
  const selectEl = document.getElementById('selectCreatorForManager');
  if (!selectEl || !selectEl.value) {
    alert('กรุณาเลือกผู้สร้างบทเรียนที่ต้องการเพิ่มเป็นผู้จัดการ');
    return;
  }
  const creatorId = parseInt(selectEl.value, 10);
  const creatorObj = allSystemCreators.find(c => c.id === creatorId);
  await promoteCreatorById(creatorId, creatorObj ? creatorObj.name : 'ผู้สร้างบทเรียน');
}

async function promoteCreatorById(creatorId, creatorName) {
  const res = await fetchJSON('/api/manager/managers', 'POST', { creator_id: creatorId });
  if (res && res.success) {
    alert('🎉 ' + res.message);
    await loadManagerDashboard();
  } else {
    alert('❌ ไม่สามารถเพิ่มผู้จัดการได้: ' + (res ? res.message : 'server error'));
  }
}

async function removeManagerById(managerId, managerName) {
  if (!confirm(`ต้องการถอนสิทธิ์ผู้จัดการของ "${managerName}" ใช่หรือไม่?`)) return;
  const res = await fetchJSON(`/api/manager/managers/${managerId}`, 'DELETE');
  if (res && res.success) {
    alert('✅ ' + res.message);
    await loadManagerDashboard();
  } else {
    alert('❌ ไม่สามารถถอนสิทธิ์ได้: ' + (res ? res.message : 'server error'));
  }
}

function computeLessonMetrics(l, wardFilter = '') {
  const statsList = Array.isArray(l.learner_stats) ? l.learner_stats : [];
  if (!wardFilter) {
    return {
      totalL: l.total_learners || 0,
      totalSec: Math.round(l.total_watch_sec || 0),
      avgSec: Math.round(l.avg_watch_sec || 0),
      preCount: l.pre_test_pass_count || 0,
      prePct: l.pre_test_pass_pct || 0,
      postCount: l.post_test_pass_count || 0,
      postPct: l.post_test_pass_pct || 0,
      avgScore: l.avg_quiz_score_pct || 0,
    };
  }
  const filtered = statsList.filter(s => (s.ward || 'สว.11') === wardFilter);
  const totalL = filtered.length;
  let totalSec = 0;
  let preCount = 0;
  let postCount = 0;
  let scoreSum = 0;
  let scoreCount = 0;
  filtered.forEach(s => {
    totalSec += (s.watched_sec || 0);
    if (s.pre_test_passed) preCount++;
    if (s.post_test_passed || s.completed) postCount++;
    if ((s.avg_score_pct || 0) > 0) {
      scoreSum += s.avg_score_pct;
      scoreCount++;
    }
  });
  return {
    totalL,
    totalSec: Math.round(totalSec),
    avgSec: totalL > 0 ? Math.round(totalSec / totalL) : 0,
    preCount,
    prePct: totalL > 0 ? (preCount / totalL) * 100 : 0,
    postCount,
    postPct: totalL > 0 ? (postCount / totalL) * 100 : 0,
    avgScore: scoreCount > 0 ? (scoreSum / scoreCount) : 0,
  };
}

function computeWardBreakdown() {
  const wardMap = {};
  WARD_OPTIONS.forEach(w => {
    wardMap[w] = {
      ward: w,
      registeredLearners: 0,
      learnerInstances: 0,
      totalWatchSec: 0,
      prePassed: 0,
      postPassed: 0,
      scoreSum: 0,
      scoreCount: 0,
    };
  });

  (allSystemLearners || []).forEach(u => {
    const w = u.ward && wardMap[u.ward] ? u.ward : 'สว.11';
    if (!wardMap[w]) {
      wardMap[w] = { ward: w, registeredLearners: 0, learnerInstances: 0, totalWatchSec: 0, prePassed: 0, postPassed: 0, scoreSum: 0, scoreCount: 0 };
    }
    wardMap[w].registeredLearners++;
  });

  (allManagerLessons || []).forEach(l => {
    (l.learner_stats || []).forEach(s => {
      const w = s.ward && wardMap[s.ward] ? s.ward : 'สว.11';
      if (!wardMap[w]) {
        wardMap[w] = { ward: w, registeredLearners: 0, learnerInstances: 0, totalWatchSec: 0, prePassed: 0, postPassed: 0, scoreSum: 0, scoreCount: 0 };
      }
      wardMap[w].learnerInstances++;
      wardMap[w].totalWatchSec += (s.watched_sec || 0);
      if (s.pre_test_passed) wardMap[w].prePassed++;
      if (s.post_test_passed || s.completed) wardMap[w].postPassed++;
      if ((s.avg_score_pct || 0) > 0) {
        wardMap[w].scoreSum += s.avg_score_pct;
        wardMap[w].scoreCount++;
      }
    });
  });

  return WARD_OPTIONS.map(w => {
    const d = wardMap[w];
    const avgWatchSec = d.learnerInstances > 0 ? (d.totalWatchSec / d.learnerInstances) : 0;
    const passPct = d.learnerInstances > 0 ? ((d.postPassed / d.learnerInstances) * 100) : 0;
    const prePct = d.learnerInstances > 0 ? ((d.prePassed / d.learnerInstances) * 100) : 0;
    const avgScorePct = d.scoreCount > 0 ? (d.scoreSum / d.scoreCount) : 0;
    return {
      ...d,
      avgWatchSec,
      passPct,
      prePct,
      avgScorePct,
    };
  });
}

function selectManagerWardFilter(ward) {
  const wardSelect = document.getElementById('managerWardFilter');
  if (wardSelect) {
    wardSelect.value = ward;
    filterManagerLessons();
  }
}

function renderManagerUI(filterText = undefined, wardFilter = undefined) {
  const container = document.getElementById('managerLessonsList');
  const reportContainer = document.getElementById('managerReportContainer');
  const searchInput = document.getElementById('managerSearchInput');
  const wardSelect = document.getElementById('managerWardFilter');

  const rawText = filterText !== undefined ? filterText : (searchInput ? searchInput.value : '');
  const activeWard = wardFilter !== undefined ? wardFilter : (wardSelect ? wardSelect.value : '');
  const query = rawText.toLowerCase().trim();

  const filteredLessons = allManagerLessons.filter(l => {
    if (!query) return true;
    const titleMatch = (l.lesson_title || '').toLowerCase().includes(query) || (l.course_title || '').toLowerCase().includes(query);
    const assignedMatch = (l.assigned_users || []).some(u =>
      (u.name || '').toLowerCase().includes(query) ||
      (u.email || '').toLowerCase().includes(query) ||
      (u.ward || '').toLowerCase().includes(query)
    );
    const statMatch = (l.learner_stats || []).some(s =>
      (s.name || '').toLowerCase().includes(query) ||
      (s.ward || '').toLowerCase().includes(query)
    );
    return titleMatch || assignedMatch || statMatch;
  });

  // 1. Render Assignment List
  if (filteredLessons.length === 0) {
    container.innerHTML = '<p style="color:var(--muted); padding:2rem; text-align:center;">ไม่พบบทเรียนที่ตรงกับคำค้นหา</p>';
  } else {
    container.innerHTML = filteredLessons.map(l => {
      const candidateLearners = activeWard
        ? allSystemLearners.filter(u => (u.ward || 'สว.11') === activeWard)
        : allSystemLearners;

      const learnerOptions = candidateLearners.length > 0
        ? candidateLearners.map(u => {
            const wardTag = `[🏥 ${escapeHtml(u.ward || 'สว.11')}]`;
            const emailTag = u.email ? ` (${escapeHtml(u.email)})` : '';
            return `<option value="${u.id}">🎓 ${escapeHtml(u.name)} ${wardTag}${emailTag}</option>`;
          }).join('')
        : `<option value="">-- ไม่มีผู้เรียนในหน่วยงาน ${escapeHtml(activeWard)} --</option>`;

      const wardOptionsHtml = WARD_OPTIONS.map(w =>
        `<option value="${escapeHtml(w)}" ${activeWard === w ? 'selected' : ''}>🏥 หน่วยงาน ${escapeHtml(w)}</option>`
      ).join('');

      const visibleAssigned = activeWard
        ? (l.assigned_users || []).filter(u => (u.ward || 'สว.11') === activeWard)
        : (l.assigned_users || []);

      const assignedChips = visibleAssigned.map(u => `
        <span class="assigned-chip">
          <span>🎓 ${escapeHtml(u.name)} <strong style="color:var(--primary); font-size:0.75rem;">[🏥 ${escapeHtml(u.ward || 'สว.11')}]</strong></span>
          <button class="assigned-chip-remove" onclick="unassignLearner(${l.lesson_id}, ${u.id})" title="ถอนการมอบหมาย">✕</button>
        </span>
      `).join('');

      return `
        <div class="manager-lesson-card">
          <div class="manager-lesson-header">
            <div>
              <h3 style="font-size:1.05rem; color:var(--text); margin-bottom:0.25rem">#${l.lesson_id} ${escapeHtml(l.lesson_title)}</h3>
              <div style="font-size:0.8rem; color:var(--muted); line-height:1.55;">
                <div>ผู้สร้าง: ${escapeHtml(l.creator_name)} • เปิดให้เรียนถึงวันที่: ${escapeHtml(l.open_period || 'เปิดตลอดเวลา')}</div>
              </div>
            </div>
            <div style="display:flex; align-items:center; gap:0.45rem; flex-wrap:wrap;">
              <span class="badge ${l.is_hidden ? 'badge-pend' : 'badge-ok'}">${l.is_hidden ? '🙈 ซ่อนอยู่' : '👁️ กำลังแสดง'}</span>
              <button type="button" class="btn btn-sm" style="background:rgba(239,68,68,0.12); color:var(--danger); border:1px solid rgba(239,68,68,0.35);" onclick="deleteManagerLesson(${l.lesson_id}, '${escapeHtml(l.lesson_title)}')" title="ลบบทเรียนนี้">
                🗑️ ลบ
              </button>
            </div>
          </div>

          <div class="manager-assign-row" style="display:flex; flex-wrap:wrap; gap:0.5rem; align-items:center;">
            <label style="font-size:0.85rem; font-weight:500;">รายบุคคล:</label>
            <select id="assignSelect_${l.lesson_id}" style="background:var(--card); color:var(--text); border:1px solid var(--border); padding:0.4rem 0.65rem; border-radius:var(--r); font-family:inherit; font-size:0.85rem; flex:1; min-width:220px;">
              ${learnerOptions}
            </select>
            <button class="btn btn-sm btn-primary" onclick="assignLearner(${l.lesson_id})">➕ มอบหมาย</button>
          </div>

          <div class="manager-assign-row" style="display:flex; flex-wrap:wrap; gap:0.5rem; align-items:center; margin-top:0.45rem; padding-top:0.45rem; border-top:1px dashed var(--border);">
            <label style="font-size:0.82rem; font-weight:600; color:var(--primary);">🏥 หน่วยงาน:</label>
            <select id="assignWardSelect_${l.lesson_id}" style="background:var(--card); color:var(--text); border:1px solid var(--border); padding:0.38rem 0.65rem; border-radius:var(--r); font-family:inherit; font-size:0.83rem; min-width:165px;">
              ${wardOptionsHtml}
            </select>
            <button class="btn btn-sm btn-outline" onclick="assignWardToLesson(${l.lesson_id})">🏥 มอบหมายทั้งหน่วย</button>
          </div>

          <div style="font-size:0.8rem; color:var(--muted); margin-top:0.5rem">
            ผู้เรียนที่ได้รับมอบหมายในบทนี้ ${activeWard ? `(เฉพาะ ${escapeHtml(activeWard)}: ${visibleAssigned.length} คน)` : `(${(l.assigned_users||[]).length} คน)`}:
            <div class="assigned-chips-wrap">
              ${assignedChips || '<span style="color:var(--dim); font-size:0.8rem">ยังไม่มีผู้เรียนที่ได้รับมอบหมาย</span>'}
            </div>
          </div>
        </div>
      `;
    }).join('');
  }

  // 2. Render Manager Overview KPIs, Ward Summary & Lesson Visual Graphs
  let grandTotalWatchSec = 0;
  let grandTotalLearnerInstances = 0;
  let grandTotalPostPassed = 0;
  let scoreSum = 0;
  let scoreLessonsCount = 0;

  allManagerLessons.forEach(l => {
    const m = computeLessonMetrics(l, activeWard);
    grandTotalWatchSec += m.totalSec;
    grandTotalLearnerInstances += m.totalL;
    grandTotalPostPassed += m.postCount;
    if (m.avgScore > 0) {
      scoreSum += m.avgScore;
      scoreLessonsCount++;
    }
  });

  const overallAvgWatchSec = grandTotalLearnerInstances > 0 ? (grandTotalWatchSec / grandTotalLearnerInstances) : 0;
  const overallPassPct = grandTotalLearnerInstances > 0 ? ((grandTotalPostPassed / grandTotalLearnerInstances) * 100) : 0;
  const overallAvgScorePct = scoreLessonsCount > 0 ? (scoreSum / scoreLessonsCount) : 0;

  // Compute Ward Breakdown across all 8 Nursing Wards
  const wardStats = computeWardBreakdown();
  const wardGraphRows = wardStats.map(w => {
    const isSelected = activeWard === w.ward;
    const passPctRound = Math.min(100, Math.round(w.passPct));
    const scorePctRound = Math.min(100, Math.round(w.avgScorePct));
    return `
      <div onclick="openWardLearnersModal('${escapeHtml(w.ward)}')" title="คลิกเพื่อดูว่าใครเรียนอะไร อยู่สถานะไหน" style="cursor:pointer; transition:transform 0.15s ease, border-color 0.15s ease; background:${isSelected ? 'rgba(2,132,199,0.08)' : 'var(--bg)'}; border:1px solid ${isSelected ? 'var(--primary)' : 'var(--border)'}; border-radius:var(--r); padding:0.75rem 1rem;">
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:0.5rem; margin-bottom:0.45rem;">
          <div style="display:flex; align-items:center; gap:0.5rem; flex-wrap:wrap;">
            <span style="font-weight:700; color:var(--text); font-size:0.95rem;">🏥 หน่วยงาน ${escapeHtml(w.ward)}</span>
            <span class="badge badge-ok" style="font-size:0.72rem;">บุคลากร ${w.registeredLearners} คน</span>
            <span class="badge badge-pend" style="font-size:0.72rem;">ลงเรียน ${w.learnerInstances} รายการ</span>
            <span class="badge" style="background:rgba(14,165,233,0.12); color:var(--primary); border:1px solid rgba(14,165,233,0.3); font-size:0.7rem;">👆 กดดูรายคน</span>
          </div>
          <div style="font-size:0.8rem; color:var(--muted);">
            เวลารวม: <strong>${(w.totalWatchSec/60).toFixed(1)} นาที</strong> |
            ผ่านหลังเรียน: <strong style="color:var(--accent)">${w.postPassed}/${w.learnerInstances} (${w.passPct.toFixed(1)}%)</strong> |
            คะแนนเฉลี่ย: <strong style="color:var(--warn)">${w.avgScorePct.toFixed(1)}%</strong>
          </div>
        </div>
        <div style="display:grid; grid-template-columns:1fr 1fr; gap:0.65rem;">
          <div>
            <div style="display:flex; justify-content:space-between; font-size:0.74rem; color:var(--muted); margin-bottom:0.2rem;">
              <span>🟩 อัตราสอบผ่านหลังเรียน (${w.postPassed}/${w.learnerInstances})</span>
              <strong>${w.passPct.toFixed(1)}%</strong>
            </div>
            <div class="mgr-bar-track">
              <div class="mgr-bar-fill emerald" style="width:${passPctRound}%"></div>
            </div>
          </div>
          <div>
            <div style="display:flex; justify-content:space-between; font-size:0.74rem; color:var(--muted); margin-bottom:0.2rem;">
              <span>🟨 คะแนนสอบเฉลี่ยรวม</span>
              <strong>${w.avgScorePct.toFixed(1)}%</strong>
            </div>
            <div class="mgr-bar-track">
              <div class="mgr-bar-fill amber" style="width:${scorePctRound}%"></div>
            </div>
          </div>
        </div>
      </div>
    `;
  }).join('');

  const wardTableRows = wardStats.map(w => `
    <tr style="${activeWard === w.ward ? 'background:rgba(2,132,199,0.08); font-weight:600;' : ''}">
      <td style="font-weight:600;">🏥 ${escapeHtml(w.ward)}</td>
      <td style="text-align:center;">${w.registeredLearners} คน</td>
      <td style="text-align:center;">${w.learnerInstances} รายการ</td>
      <td style="text-align:center;">${(w.totalWatchSec/60).toFixed(1)} นาที (${Math.round(w.totalWatchSec)}s)</td>
      <td style="text-align:center;">${(w.avgWatchSec/60).toFixed(1)} นาที (${Math.round(w.avgWatchSec)}s)</td>
      <td style="text-align:center;">${w.prePassed}/${w.learnerInstances} (${w.prePct.toFixed(1)}%)</td>
      <td style="text-align:center; color:#34d399; font-weight:600;">${w.postPassed}/${w.learnerInstances} (${w.passPct.toFixed(1)}%)</td>
      <td style="text-align:center; color:#facc15; font-weight:600;">${w.avgScorePct.toFixed(1)}%</td>
    </tr>
  `).join('');

  const lessonGraphCards = filteredLessons.map(l => {
    const m          = computeLessonMetrics(l, activeWard);
    const totalL     = m.totalL;
    const totalSec   = m.totalSec;
    const totalMin   = (totalSec / 60).toFixed(1);
    const avgSec     = m.avgSec;
    const avgMin     = (avgSec / 60).toFixed(1);
    const reqSec     = Math.round(l.duration_sec || 60);
    const watchPct   = reqSec > 0 ? Math.min(100, Math.round((avgSec / reqSec) * 100)) : 0;

    const preCount   = m.preCount;
    const prePct     = Math.min(100, Math.round(m.prePct));
    const postCount  = m.postCount;
    const postPct    = Math.min(100, Math.round(m.postPct));
    const avgScore   = Math.min(100, Math.round(m.avgScore));

    return `
      <div class="mgr-graph-card" style="cursor:pointer; transition:transform 0.15s ease, border-color 0.15s ease;" onclick="openLessonLearnersModal(${l.lesson_id}, '${escapeHtml(l.lesson_title)}')" title="คลิกเพื่อโหลดสถิติผู้เรียนแต่ละคน">
        <div class="mgr-graph-hdr">
          <div>
            <div class="mgr-graph-title">#${l.lesson_id} ${escapeHtml(l.lesson_title)}</div>
            <div class="mgr-graph-sub">เวลาเรียนขั้นต่ำ: ${reqSec} วินาที ${activeWard ? `• เฉพาะหน่วยงาน: ${escapeHtml(activeWard)}` : ''}</div>
          </div>
          <div style="display:flex; align-items:center; gap:0.4rem; flex-wrap:wrap;">
            <span class="badge badge-ok">👥 ผู้เรียน ${totalL} คน</span>
            <span class="badge" style="background:rgba(14,165,233,0.12); color:var(--primary); border:1px solid rgba(14,165,233,0.3); font-size:0.72rem;">👆 กดดูรายคน</span>
          </div>
        </div>

        <div class="mgr-metrics-strip">
          <div class="mgr-metric-pill">
            <span class="m-lbl">⏱️ ระยะเวลาเรียนรวม</span>
            <span class="m-val">${totalMin} นาที (${totalSec}s)</span>
          </div>
          <div class="mgr-metric-pill">
            <span class="m-lbl">⌛ เวลาเรียนเฉลี่ย/คน</span>
            <span class="m-val">${avgMin} นาที (${avgSec}s)</span>
          </div>
          <div class="mgr-metric-pill">
            <span class="m-lbl">✅ ทำแบบทดสอบผ่าน</span>
            <span class="m-val" style="color:#34d399">${postCount} / ${totalL} คน (${m.postPct.toFixed(1)}%)</span>
          </div>
          <div class="mgr-metric-pill">
            <span class="m-lbl">🎯 คะแนนสอบเฉลี่ย</span>
            <span class="m-val" style="color:#facc15">${m.avgScore.toFixed(1)}%</span>
          </div>
          <div class="mgr-metric-pill">
            <span class="m-lbl">⭐ คะแนนพึงพอใจ</span>
            <span class="m-val" style="color:#f59e0b">${(l.average_rating || 0).toFixed(1)} / 5 (${l.total_ratings || 0} คน)</span>
          </div>
        </div>

        <div class="mgr-bars-wrap">
          <div class="mgr-bar-row">
            <div class="mgr-bar-label">
              <span>🟦 เวลาเรียนเฉลี่ยเทียบเกณฑ์ (${avgSec}s / ${reqSec}s)</span>
              <strong>${watchPct}%</strong>
            </div>
            <div class="mgr-bar-track">
              <div class="mgr-bar-fill blue" style="width:${watchPct}%"></div>
            </div>
          </div>

          <div class="mgr-bar-row">
            <div class="mgr-bar-label">
              <span>🟪 แบบทดสอบก่อนเรียนผ่าน (${preCount}/${totalL} คน)</span>
              <strong>${m.prePct.toFixed(1)}%</strong>
            </div>
            <div class="mgr-bar-track">
              <div class="mgr-bar-fill indigo" style="width:${prePct}%"></div>
            </div>
          </div>

          <div class="mgr-bar-row">
            <div class="mgr-bar-label">
              <span>🟩 แบบทดสอบหลังเรียนผ่าน / สำเร็จ (${postCount}/${totalL} คน)</span>
              <strong>${m.postPct.toFixed(1)}%</strong>
            </div>
            <div class="mgr-bar-track">
              <div class="mgr-bar-fill emerald" style="width:${postPct}%"></div>
            </div>
          </div>

          <div class="mgr-bar-row">
            <div class="mgr-bar-label">
              <span>🟨 คะแนนทำแบบทดสอบเฉลี่ยรวม (ร้อยละ)</span>
              <strong>${m.avgScore.toFixed(1)}%</strong>
            </div>
            <div class="mgr-bar-track">
              <div class="mgr-bar-fill amber" style="width:${avgScore}%"></div>
            </div>
          </div>
        </div>
      </div>
    `;
  }).join('');

  const summaryTableRows = filteredLessons.map(l => {
    const m        = computeLessonMetrics(l, activeWard);
    const totalL   = m.totalL;
    const totalSec = m.totalSec;
    const avgSec   = m.avgSec;
    return `
      <tr>
        <td style="font-weight:600;">#${l.lesson_id} ${escapeHtml(l.lesson_title)}</td>
        <td style="text-align:center;">${totalL} คน</td>
        <td style="text-align:center;">${(totalSec/60).toFixed(1)} นาที (${totalSec}s)</td>
        <td style="text-align:center;">${(avgSec/60).toFixed(1)} นาที (${avgSec}s)</td>
        <td style="text-align:center;">${m.preCount}/${totalL} (${m.prePct.toFixed(1)}%)</td>
        <td style="text-align:center; color:#34d399; font-weight:600;">${m.postCount}/${totalL} (${m.postPct.toFixed(1)}%)</td>
        <td style="text-align:center; color:#facc15; font-weight:600;">${m.avgScore.toFixed(1)}%</td>
      </tr>
    `;
  }).join('');

  reportContainer.innerHTML = `
    ${activeWard ? `
      <div style="background:rgba(2,132,199,0.1); border:1px solid var(--primary); border-radius:var(--r); padding:0.65rem 1rem; margin-bottom:1.25rem; display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:0.5rem;">
        <span style="font-size:0.9rem; font-weight:600; color:var(--primary);">🏥 กำลังแสดงข้อมูลกรองเฉพาะหน่วยงาน: "${escapeHtml(activeWard)}"</span>
        <button class="btn btn-sm btn-outline" onclick="selectManagerWardFilter('')">แสดงทุกหน่วยงาน</button>
      </div>
    ` : ''}

    <!-- Overall KPI Cards -->
    <div class="stats-grid" style="margin-bottom:1.5rem;">
      <div class="stat-box">
        <div class="stat-num" style="color:#38bdf8">${(grandTotalWatchSec/60).toFixed(1)} นาที</div>
        <div class="stat-label">ระยะเวลาเรียนรวมทุกบท (${Math.round(grandTotalWatchSec)}s)</div>
      </div>
      <div class="stat-box">
        <div class="stat-num" style="color:#818cf8">${(overallAvgWatchSec/60).toFixed(1)} นาที</div>
        <div class="stat-label">ระยะเวลาเรียนเฉลี่ย/คน (${Math.round(overallAvgWatchSec)}s)</div>
      </div>
      <div class="stat-box">
        <div class="stat-num" style="color:#34d399">${overallPassPct.toFixed(1)}%</div>
        <div class="stat-label">อัตราทำแบบทดสอบผ่าน (${grandTotalPostPassed}/${grandTotalLearnerInstances} คน)</div>
      </div>
      <div class="stat-box">
        <div class="stat-num" style="color:#facc15">${overallAvgScorePct.toFixed(1)}%</div>
        <div class="stat-label">คะแนนทำแบบทดสอบเฉลี่ยรวม</div>
      </div>
    </div>

    <!-- Ward Summary Section (หอสงฆ์, สว.11, สว.12, 8A, 8B, 9A, 9B, 9C) -->
    <div style="background:var(--card); border:1px solid var(--border); border-radius:var(--r); padding:1.25rem; margin-bottom:1.5rem;">
      <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:0.5rem; margin-bottom:1rem;">
        <div>
          <h4 style="margin:0; font-size:1.05rem; color:var(--text);">🏥 สรุปสถิติแยกตามหน่วยงาน (หอสงฆ์, สว.11, สว.12, 8A, 8B, 9A, 9B, 9C)</h4>
          <p style="margin:0.2rem 0 0; font-size:0.8rem; color:var(--muted);">เปรียบเทียบจำนวนบุคลากร ระยะเวลาเรียน และอัตราการสอบผ่านรายหอผู้ป่วย</p>
        </div>
      </div>
      <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(340px, 1fr)); gap:0.75rem; margin-bottom:1.1rem;">
        ${wardGraphRows}
      </div>
      <div class="report-table-wrap">
        <table class="report-table">
          <thead>
            <tr>
              <th>หน่วยงาน (หอผู้ป่วย)</th>
              <th style="text-align:center;">บุคลากรในระบบ</th>
              <th style="text-align:center;">รายการเข้าเรียน</th>
              <th style="text-align:center;">เวลาเรียนรวม</th>
              <th style="text-align:center;">เวลาเรียนเฉลี่ย</th>
              <th style="text-align:center;">ผ่านก่อนเรียน (%)</th>
              <th style="text-align:center;">ผ่านหลังเรียน (%)</th>
              <th style="text-align:center;">คะแนนเฉลี่ย (%)</th>
            </tr>
          </thead>
          <tbody>
            ${wardTableRows}
          </tbody>
        </table>
      </div>
    </div>

    <!-- Visual Bar Graphs per Lesson -->
    <div class="mgr-graphs-grid">
      ${lessonGraphCards || '<p style="color:var(--muted); text-align:center; padding:2rem;">ยังไม่มีข้อมูลบทเรียน</p>'}
    </div>

    <!-- Summary Table -->
    <div class="report-table-wrap" style="margin-top:1.5rem;">
      <table class="report-table">
        <thead>
          <tr>
            <th>บทเรียน</th>
            <th style="text-align:center;">ผู้เรียน</th>
            <th style="text-align:center;">เวลาเรียนรวม</th>
            <th style="text-align:center;">เวลาเรียนเฉลี่ย</th>
            <th style="text-align:center;">ผ่านก่อนเรียน (%)</th>
            <th style="text-align:center;">ผ่านหลังเรียน (%)</th>
            <th style="text-align:center;">คะแนนเฉลี่ย (%)</th>
          </tr>
        </thead>
        <tbody>
          ${summaryTableRows || '<tr><td colspan="7" style="text-align:center; padding:2rem;">ยังไม่มีข้อมูลสถิติ</td></tr>'}
        </tbody>
      </table>
    </div>
  `;
}

function filterManagerLessons() {
  const input = document.getElementById('managerSearchInput');
  const wardSelect = document.getElementById('managerWardFilter');
  renderManagerUI(
    input ? input.value : '',
    wardSelect ? wardSelect.value : ''
  );
}

async function assignLearner(lessonId) {
  const select = document.getElementById(`assignSelect_${lessonId}`);
  if (!select) return;
  const userId = parseInt(select.value, 10);
  if (!userId) {
    alert('กรุณาเลือกผู้เรียน');
    return;
  }

  const res = await fetchJSON('/api/manager/assign', 'POST', { lesson_id: lessonId, user_id: userId });
  if (res && res.success) {
    alert('🎉 ' + res.message);
    await loadManagerDashboard();
  } else {
    alert('❌ ไม่สามารถมอบหมายได้: ' + (res ? res.message : 'server error'));
  }
}

async function assignWardToLesson(lessonId) {
  const select = document.getElementById(`assignWardSelect_${lessonId}`);
  if (!select || !select.value) {
    alert('กรุณาเลือกหน่วยงาน');
    return;
  }
  const ward = select.value;
  const res = await fetchJSON('/api/manager/assign', 'POST', { lesson_id: lessonId, ward });
  if (res && res.success) {
    alert('🎉 ' + res.message);
    await loadManagerDashboard();
  } else {
    alert('❌ ไม่สามารถมอบหมายหน่วยงานได้: ' + (res ? res.message : 'server error'));
  }
}

async function unassignLearner(lessonId, userId) {
  if (!confirm('ต้องการถอนการมอบหมายผู้เรียนท่านนี้ใช่หรือไม่?')) return;
  const res = await fetchJSON(`/api/manager/assign?lesson_id=${lessonId}&user_id=${userId}`, 'DELETE');
  if (res && res.success) {
    await loadManagerDashboard();
  } else {
    alert('❌ ไม่สามารถถอนการมอบหมายได้: ' + (res ? res.message : 'server error'));
  }
}

async function deleteManagerLesson(lessonId, title) {
  if (!confirm(`⚠️ ต้องการลบบทเรียน "${title}" ออกจากระบบถาวรใช่หรือไม่?\n(ข้อมูลการมอบหมายและสถิติของบทเรียนนี้จะถูกลบออกด้วย)`)) return;
  const res = await fetchJSON(`/api/lessons/${lessonId}`, 'DELETE');
  if (res && res.success) {
    alert('✅ ' + (res.message || 'ลบบทเรียนเรียบร้อยแล้ว'));
    await loadManagerDashboard();
  } else {
    alert('❌ ไม่สามารถลบบทเรียนได้: ' + (res ? res.message : 'server error'));
  }
}

function printManagerReport() {
  const wardSelect = document.getElementById('managerWardFilter');
  const activeWard = wardSelect ? wardSelect.value : '';

  let grandTotalWatchSec = 0;
  let grandTotalLearnerInstances = 0;
  let grandTotalPostPassed = 0;
  let scoreSum = 0;
  let scoreLessonsCount = 0;

  allManagerLessons.forEach(l => {
    const m = computeLessonMetrics(l, activeWard);
    grandTotalWatchSec += m.totalSec;
    grandTotalLearnerInstances += m.totalL;
    grandTotalPostPassed += m.postCount;
    if (m.avgScore > 0) {
      scoreSum += m.avgScore;
      scoreLessonsCount++;
    }
  });

  const overallAvgWatchSec = grandTotalLearnerInstances > 0 ? (grandTotalWatchSec / grandTotalLearnerInstances) : 0;
  const overallPassPct = grandTotalLearnerInstances > 0 ? ((grandTotalPostPassed / grandTotalLearnerInstances) * 100) : 0;
  const overallAvgScorePct = scoreLessonsCount > 0 ? (scoreSum / scoreLessonsCount) : 0;

  const managerName = currentUser ? (currentUser.name || currentUser.email || 'ผู้จัดการระบบ') : 'ผู้จัดการระบบ';
  const managerEmail = currentUser && currentUser.email ? ` (${currentUser.email})` : '';
  const issuedAtStr = new Date().toLocaleDateString('th-TH', { year: 'numeric', month: 'long', day: 'numeric', hour: '2-digit', minute: '2-digit' });

  const wardStats = computeWardBreakdown();
  const printWardRows = wardStats.map(w => `
    <tr style="border-bottom:1px solid #cbd5e1; ${activeWard === w.ward ? 'background:#e0f2fe;' : ''}">
      <td style="padding:7px 10px; font-weight:700; color:#0f172a;">🏥 ${escapeHtml(w.ward)}</td>
      <td style="padding:7px 10px; text-align:center; color:#334155;">${w.registeredLearners} คน</td>
      <td style="padding:7px 10px; text-align:center; color:#334155;">${w.learnerInstances} รายการ</td>
      <td style="padding:7px 10px; text-align:center; color:#334155;">${(w.totalWatchSec/60).toFixed(1)} นาที (${Math.round(w.totalWatchSec)}s)</td>
      <td style="padding:7px 10px; text-align:center; color:#334155;">${(w.avgWatchSec/60).toFixed(1)} นาที (${Math.round(w.avgWatchSec)}s)</td>
      <td style="padding:7px 10px; text-align:center; color:#4f46e5; font-weight:600;">${w.prePassed}/${w.learnerInstances} (${w.prePct.toFixed(1)}%)</td>
      <td style="padding:7px 10px; text-align:center; color:#059669; font-weight:700;">${w.postPassed}/${w.learnerInstances} (${w.passPct.toFixed(1)}%)</td>
      <td style="padding:7px 10px; text-align:center; color:#b45309; font-weight:700;">${w.avgScorePct.toFixed(1)}%</td>
    </tr>
  `).join('');

  const printGraphCards = allManagerLessons.map(l => {
    const m         = computeLessonMetrics(l, activeWard);
    const totalL    = m.totalL;
    const totalSec  = m.totalSec;
    const totalMin  = (totalSec / 60).toFixed(1);
    const avgSec    = m.avgSec;
    const avgMin    = (avgSec / 60).toFixed(1);
    const reqSec    = Math.round(l.duration_sec || 60);
    const watchPct  = reqSec > 0 ? Math.min(100, Math.round((avgSec / reqSec) * 100)) : 0;
    const preCount  = m.preCount;
    const prePct    = Math.min(100, Math.round(m.prePct));
    const postCount = m.postCount;
    const postPct   = Math.min(100, Math.round(m.postPct));
    const avgScore  = Math.min(100, Math.round(m.avgScore));

    return `
      <div style="border:1px solid #cbd5e1; border-radius:10px; padding:14px 16px; background:#ffffff; page-break-inside:avoid;">
        <div style="display:flex; justify-content:space-between; align-items:flex-start; border-bottom:1px solid #e2e8f0; padding-bottom:8px; margin-bottom:10px;">
          <div>
            <div style="font-size:13.5px; font-weight:700; color:#0f172a;">#${l.lesson_id} ${escapeHtml(l.lesson_title)}</div>
            <div style="font-size:11px; color:#64748b;">เวลารวม: ${totalMin} นาที (${totalSec}s) | เฉลี่ย: ${avgMin} นาที (${avgSec}s/คน) | สอบผ่าน: ${postCount}/${totalL} คน (${m.postPct.toFixed(1)}%)</div>
          </div>
          <span style="background:#eff6ff; color:#1d4ed8; border:1px solid #bfdbfe; padding:2px 10px; border-radius:999px; font-size:11px; font-weight:700; white-space:nowrap;">ผู้เรียน ${totalL} คน</span>
        </div>

        <!-- 4 Horizontal Print-Ready Bar Charts -->
        <div style="display:grid; grid-template-columns:1fr 1fr; gap:8px 16px; font-size:11px;">
          <div>
            <div style="display:flex; justify-content:space-between; margin-bottom:3px; color:#334155;">
              <span>เวลาเรียนเฉลี่ยเทียบเกณฑ์ (${avgSec}s/${reqSec}s)</span>
              <strong style="color:#0284c7;">${watchPct}%</strong>
            </div>
            <div style="height:8px; background:#e2e8f0; border-radius:999px; overflow:hidden;">
              <div style="width:${watchPct}%; height:100%; background:#0284c7; border-radius:999px;"></div>
            </div>
          </div>

          <div>
            <div style="display:flex; justify-content:space-between; margin-bottom:3px; color:#334155;">
              <span>ผ่านแบบทดสอบก่อนเรียน (${preCount}/${totalL} คน)</span>
              <strong style="color:#4f46e5;">${m.prePct.toFixed(1)}%</strong>
            </div>
            <div style="height:8px; background:#e2e8f0; border-radius:999px; overflow:hidden;">
              <div style="width:${prePct}%; height:100%; background:#4f46e5; border-radius:999px;"></div>
            </div>
          </div>

          <div>
            <div style="display:flex; justify-content:space-between; margin-bottom:3px; color:#334155;">
              <span>ผ่านแบบทดสอบหลังเรียน (${postCount}/${totalL} คน)</span>
              <strong style="color:#059669;">${m.postPct.toFixed(1)}%</strong>
            </div>
            <div style="height:8px; background:#e2e8f0; border-radius:999px; overflow:hidden;">
              <div style="width:${postPct}%; height:100%; background:#10b981; border-radius:999px;"></div>
            </div>
          </div>

          <div>
            <div style="display:flex; justify-content:space-between; margin-bottom:3px; color:#334155;">
              <span>คะแนนทำแบบทดสอบเฉลี่ยรวม</span>
              <strong style="color:#b45309;">${m.avgScore.toFixed(1)}%</strong>
            </div>
            <div style="height:8px; background:#e2e8f0; border-radius:999px; overflow:hidden;">
              <div style="width:${avgScore}%; height:100%; background:#eab308; border-radius:999px;"></div>
            </div>
          </div>
        </div>
      </div>
    `;
  }).join('');

  const printTableRows = allManagerLessons.map(l => {
    const m        = computeLessonMetrics(l, activeWard);
    const totalL   = m.totalL;
    const totalSec = m.totalSec;
    const avgSec   = m.avgSec;
    return `
      <tr style="border-bottom:1px solid #cbd5e1;">
        <td style="padding:8px 10px; font-weight:600; color:#0f172a;">#${l.lesson_id} ${escapeHtml(l.lesson_title)}</td>
        <td style="padding:8px 10px; text-align:center; color:#334155;">${totalL} คน</td>
        <td style="padding:8px 10px; text-align:center; color:#334155;">${(totalSec/60).toFixed(1)} นาที (${totalSec}s)</td>
        <td style="padding:8px 10px; text-align:center; color:#334155;">${(avgSec/60).toFixed(1)} นาที (${avgSec}s)</td>
        <td style="padding:8px 10px; text-align:center; color:#4f46e5; font-weight:600;">${m.preCount}/${totalL} (${m.prePct.toFixed(1)}%)</td>
        <td style="padding:8px 10px; text-align:center; color:#059669; font-weight:700;">${m.postCount}/${totalL} (${m.postPct.toFixed(1)}%)</td>
        <td style="padding:8px 10px; text-align:center; color:#b45309; font-weight:700;">${m.avgScore.toFixed(1)}%</td>
      </tr>
    `;
  }).join('');

  const reportHTML = `
    <div style="max-width:840px; margin:0 auto; background:#ffffff; color:#0f172a; font-family:'Prompt',sans-serif; border:1px solid #cbd5e1; border-radius:14px; overflow:hidden; box-shadow:0 8px 24px rgba(15,23,42,0.12);">
      <!-- Executive Report Header -->
      <div style="background:linear-gradient(135deg, #0f172a 0%, #1e3a8a 100%); color:#ffffff; padding:22px 28px; display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:12px; border-bottom:4px solid #38bdf8;">
        <div>
          <div style="font-size:10.5px; color:#7dd3fc; font-weight:700; letter-spacing:1px;">⛓️ รายงานสถิติการเรียนรู้อิเล็กทรอนิกส์</div>
          <h1 style="margin:4px 0 4px; font-size:20px; font-weight:700; color:#ffffff;">รายงานวิเคราะห์สถิติการเรียนรู้ ${activeWard ? `- หน่วยงาน ${escapeHtml(activeWard)}` : '- ทุกหน่วยงาน'}</h1>
          <div style="font-size:11.5px; color:#bae6fd; line-height:1.5;">${ISSUING_ORG_MULTILINE_HTML}</div>
          <div style="font-size:11.5px; color:#cbd5e1; margin-top:4px;">ผู้จัดการผู้ออกรายงาน: <strong>${escapeHtml(managerName)}${escapeHtml(managerEmail)}</strong></div>
        </div>
        <div style="text-align:right; font-size:11.5px; color:#e2e8f0; background:rgba(255,255,255,0.08); padding:8px 14px; border-radius:8px; border:1px solid rgba(255,255,255,0.18);">
          <div><strong>วันที่พิมพ์รายงาน:</strong></div>
          <div>${escapeHtml(issuedAtStr)}</div>
        </div>
      </div>

      <div style="padding:22px 26px;">
        <!-- 4 Executive KPI Boxes -->
        <div style="display:grid; grid-template-columns:repeat(4, 1fr); gap:10px; margin-bottom:20px;">
          <div style="background:#f0f9ff; border:1px solid #bae6fd; border-radius:10px; padding:10px 12px; text-align:center;">
            <div style="font-size:18px; font-weight:700; color:#0369a1;">${(grandTotalWatchSec/60).toFixed(1)} นาที</div>
            <div style="font-size:11px; color:#475569;">ระยะเวลาเรียนรวม (${Math.round(grandTotalWatchSec)}s)</div>
          </div>
          <div style="background:#eef2ff; border:1px solid #c7d2fe; border-radius:10px; padding:10px 12px; text-align:center;">
            <div style="font-size:18px; font-weight:700; color:#4338ca;">${(overallAvgWatchSec/60).toFixed(1)} นาที</div>
            <div style="font-size:11px; color:#475569;">เวลาเรียนเฉลี่ย/คน (${Math.round(overallAvgWatchSec)}s)</div>
          </div>
          <div style="background:#ecfdf5; border:1px solid #a7f3d0; border-radius:10px; padding:10px 12px; text-align:center;">
            <div style="font-size:18px; font-weight:700; color:#047857;">${overallPassPct.toFixed(1)}%</div>
            <div style="font-size:11px; color:#475569;">อัตราทำแบบทดสอบผ่าน (${grandTotalPostPassed}/${grandTotalLearnerInstances})</div>
          </div>
          <div style="background:#fefce8; border:1px solid #fde047; border-radius:10px; padding:10px 12px; text-align:center;">
            <div style="font-size:18px; font-weight:700; color:#b45309;">${overallAvgScorePct.toFixed(1)}%</div>
            <div style="font-size:11px; color:#475569;">คะแนนแบบทดสอบเฉลี่ยรวม</div>
          </div>
        </div>

        <!-- Section 1: Ward Breakdown Table -->
        <h3 style="font-size:14px; font-weight:700; color:#1e3a8a; margin:0 0 10px; border-left:4px solid #0284c7; padding-left:8px;">
          1. ตารางสรุปสถิติแยกตามหน่วยงาน (หอสงฆ์, สว.11, สว.12, 8A, 8B, 9A, 9B, 9C)
        </h3>
        <table style="width:100%; border-collapse:collapse; font-size:11.5px; border:1px solid #cbd5e1; margin-bottom:22px;">
          <thead>
            <tr style="background:#f1f5f9; color:#1e293b; border-bottom:2px solid #cbd5e1;">
              <th style="padding:8px 10px; text-align:left;">หน่วยงาน</th>
              <th style="padding:8px 10px; text-align:center;">บุคลากร</th>
              <th style="padding:8px 10px; text-align:center;">รายการเรียน</th>
              <th style="padding:8px 10px; text-align:center;">เวลาเรียนรวม</th>
              <th style="padding:8px 10px; text-align:center;">เวลาเรียนเฉลี่ย</th>
              <th style="padding:8px 10px; text-align:center;">ผ่านก่อนเรียน (%)</th>
              <th style="padding:8px 10px; text-align:center;">ผ่านหลังเรียน (%)</th>
              <th style="padding:8px 10px; text-align:center;">คะแนนเฉลี่ย (%)</th>
            </tr>
          </thead>
          <tbody>
            ${printWardRows}
          </tbody>
        </table>

        <!-- Section 2: Visual Bar Graphs -->
        <h3 style="font-size:14px; font-weight:700; color:#1e3a8a; margin:0 0 10px; border-left:4px solid #2563eb; padding-left:8px;">
          2. กราฟแสดงภาพรวมผลการเรียนรู้รายบทเรียน
        </h3>
        <div style="display:flex; flex-direction:column; gap:10px; margin-bottom:22px;">
          ${printGraphCards || '<div style="text-align:center; padding:16px; color:#64748b;">ไม่มีข้อมูลบทเรียน</div>'}
        </div>

        <!-- Section 3: Summary Table -->
        <h3 style="font-size:14px; font-weight:700; color:#1e3a8a; margin:0 0 10px; border-left:4px solid #2563eb; padding-left:8px;">
          3. ตารางสรุปสถิติภาพรวมรายบทเรียน
        </h3>
        <table style="width:100%; border-collapse:collapse; font-size:12px; border:1px solid #cbd5e1; margin-bottom:24px;">
          <thead>
            <tr style="background:#f1f5f9; color:#1e293b; border-bottom:2px solid #cbd5e1;">
              <th style="padding:9px 10px; text-align:left;">บทเรียน</th>
              <th style="padding:9px 10px; text-align:center;">ผู้เรียน</th>
              <th style="padding:9px 10px; text-align:center;">เวลาเรียนรวม</th>
              <th style="padding:9px 10px; text-align:center;">เวลาเรียนเฉลี่ย</th>
              <th style="padding:9px 10px; text-align:center;">ผ่านก่อนเรียน (%)</th>
              <th style="padding:9px 10px; text-align:center;">ผ่านหลังเรียน (%)</th>
              <th style="padding:9px 10px; text-align:center;">คะแนนเฉลี่ย (%)</th>
            </tr>
          </thead>
          <tbody>
            ${printTableRows || '<tr><td colspan="7" style="text-align:center; padding:16px;">ไม่มีข้อมูลสถิติ</td></tr>'}
          </tbody>
        </table>

        <!-- Footer & Signature -->
        <div style="display:flex; justify-content:space-between; align-items:flex-end; border-top:1px dashed #cbd5e1; padding-top:18px; margin-top:12px; font-size:11.5px; color:#475569;">
          <div style="line-height:1.55;">
            <div><strong>สถานะความถูกต้องของข้อมูล:</strong> ตรวจสอบและยืนยันจากบันทึกธุรกรรมบนบล็อกเชน</div>
            <div style="color:#64748b; font-size:10.5px;">${ISSUING_ORG_MULTILINE_HTML}</div>
          </div>
          <div style="text-align:center; min-width:220px;">
            <div style="border-bottom:1px dotted #64748b; height:24px; margin-bottom:4px;"></div>
            <div>(${escapeHtml(managerName)})</div>
            <div style="font-size:10.5px; color:#64748b;">ผู้จัดการ / ผู้รับรองรายงานสถิติ</div>
          </div>
        </div>
      </div>
    </div>
  `;

  openPrintPreviewModal('รายงานวิเคราะห์สถิติการเรียนรู้', reportHTML);
}

async function emailManagerReport() {
  const res = await fetchJSON('/api/manager/report/email', 'POST');
  if (res && res.success) {
    alert('🎉 ' + res.message);
  } else {
    alert('❌ ไม่สามารถส่งอีเมลได้: ' + (res ? res.message : 'server error'));
  }
}

// ============================================================
//  SHARED: LESSON PLAYER MODAL & EXAMINATIONS
// ============================================================
async function openLesson(lessonId) {
  const data = await fetchJSON(`/api/lessons/${lessonId}`);
  if (!data || !data.success) {
    alert('❌ ไม่สามารถเข้าถึงบทเรียน: ' + (data ? data.message : 'server error'));
    return;
  }

  currentLesson = data.lesson;
  const progress = data.progress || {};

  set('modalBadge',   `บทที่ ${currentLesson.sort_order}`);
  set('modalTitle',   currentLesson.title);
  set('modalSummary', currentLesson.summary || currentLesson.description || '');

  lessonDuration = currentLesson.duration_sec || 60;
  const reqMin = String(Math.floor(lessonDuration / 60)).padStart(2, '0');
  const reqSec = String(Math.floor(lessonDuration % 60)).padStart(2, '0');
  set('timeRequired', `${reqMin}:${reqSec}`);
  set('lockedReq',    `${reqMin}:${reqSec}`);
  set('modalTimeBadge', `⏱️ เวลาเรียนขั้นต่ำ: ${reqMin}:${reqSec}`);

  const docLink = document.getElementById('modalDocLink');
  if (currentLesson.document_url && currentLesson.document_url.trim()) {
    docLink.href = currentLesson.document_url;
    docLink.style.display = 'inline-flex';
  } else {
    docLink.style.display = 'none';
  }

  serverAcceptedSec = progress.last_watched_sec || 0;
  currentSec        = serverAcceptedSec;
  window._lessonCompletedNotified = !!progress.is_completed;
  updateTimerDisplay(currentSec);

  currentQuizzes = {};
  if (currentLesson.quizzes) {
    currentLesson.quizzes.forEach(q => {
      currentQuizzes[q.type] = q.id;
    });
  }

  const hasPreQuiz = !currentLesson.disable_pre_test && !!currentQuizzes['lesson_pre_test'];
  const hasPostQuiz = !currentLesson.disable_post_test && !!currentQuizzes['lesson_post_test'];
  const tabPreBtn = document.getElementById('tabPre');
  if (tabPreBtn) {
    tabPreBtn.classList.toggle('hidden', !hasPreQuiz);
  }
  const tabPostBtn = document.getElementById('tabPost');
  if (tabPostBtn) {
    tabPostBtn.classList.toggle('hidden', !hasPostQuiz);
  }

  const postUnlocked = progress.video_watched || (currentSec >= lessonDuration);
  updatePostLockUI(postUnlocked);

  const newVideoId = extractYoutubeId(currentLesson.youtube_url);
  loadLessonRatingWidget(lessonId);
  initOrLoadYouTubePlayer(newVideoId);

  // Switch to initial tab: If no pre-test or pre-test already passed, show video
  if (!hasPreQuiz || progress.pre_test_passed) {
    switchTab('video');
  } else {
    switchTab('pre');
  }

  document.getElementById('lessonModal').classList.remove('hidden');
}

function closeModal() {
  document.getElementById('lessonModal').classList.add('hidden');
  pauseVideo();
  if (ytPlayer && typeof ytPlayer.stopVideo === 'function') {
    ytPlayer.stopVideo();
  }
}

function onModalBgClick(e) {
  if (e.target.id === 'lessonModal') closeModal();
}

function switchTab(tab) {
  activeTab = tab;
  ['pre', 'video', 'post'].forEach(t => {
    const btn   = document.getElementById(`tab${capitalize(t)}`);
    const panel = document.getElementById(`view${capitalize(t)}`);
    if (t === tab) {
      btn.classList.add('active');
      panel.classList.remove('hidden');
    } else {
      btn.classList.remove('active');
      panel.classList.add('hidden');
    }
  });

  if (tab === 'pre')  loadQuiz('lesson_pre_test',  'preQuizArea');
  if (tab === 'post') loadQuiz('lesson_post_test', 'postQuizArea');
}

// ── YouTube IFrame API Sync ──
function initOrLoadYouTubePlayer(videoId) {
  ytVideoId = videoId;
  if (!videoId) return;

  if (ytPlayer && typeof ytPlayer.loadVideoById === 'function') {
    try {
      ytPlayer.loadVideoById(videoId);
      return;
    } catch (e) {
      console.warn('Error loading video:', e);
    }
  }

  if (window.YT && window.YT.Player) {
    createYTPlayer(videoId);
  } else {
    const checkYT = setInterval(() => {
      if (window.YT && window.YT.Player) {
        clearInterval(checkYT);
        createYTPlayer(videoId);
      }
    }, 200);
  }
}

function createYTPlayer(videoId) {
  try {
    ytPlayer = new YT.Player('ytPlayer', {
      height: '100%',
      width: '100%',
      videoId: videoId,
      playerVars: {
        playsinline: 1,
        rel: 0,
        modestbranding: 1
      },
      events: {
        'onStateChange': onPlayerStateChange
      }
    });
  } catch (err) {
    console.error('Failed to create YouTube Player:', err);
  }
}

function onPlayerStateChange(event) {
  if (event.data === YT.PlayerState.PLAYING) {
    if (!isPlaying) startTimer();
  } else if (event.data === YT.PlayerState.PAUSED || event.data === YT.PlayerState.ENDED) {
    if (isPlaying) pauseVideo();
  }
}

function togglePlay() {
  if (isPlaying) {
    pauseVideo();
    if (ytPlayer && typeof ytPlayer.pauseVideo === 'function') ytPlayer.pauseVideo();
  } else {
    startTimer();
    if (ytPlayer && typeof ytPlayer.playVideo === 'function') ytPlayer.playVideo();
  }
}

function startTimer() {
  if (isPlaying) return;
  isPlaying = true;
  updatePlayButton(true);

  videoInterval = setInterval(async () => {
    currentSec++;
    updateTimerDisplay(currentSec);

    if (currentSec % 5 === 0 || currentSec >= lessonDuration) {
      const res = await sendHeartbeat(currentSec);
      if (res && res.success && res.progress) {
        serverAcceptedSec = res.progress.last_watched_sec;
        if (res.progress.is_completed && !window._lessonCompletedNotified && (currentLesson.disable_post_test || !currentQuizzes['lesson_post_test'])) {
          window._lessonCompletedNotified = true;
          const pill = document.getElementById('timerPill');
          if (pill) {
            pill.className = 'timer-pill passed';
            pill.innerText = '✅ เรียนครบเวลาแล้ว — ผ่านบทเรียนและออกใบประกาศ!';
          }
          if (typeof loadCourseData === 'function') loadCourseData();
        }
      }
    }

    if (currentSec >= lessonDuration) {
      updatePostLockUI(true);
    }
  }, 1000);
}

function pauseVideo() {
  isPlaying = false;
  clearInterval(videoInterval);
  videoInterval = null;
  updatePlayButton(false);
}

function updatePlayButton(playing) {
  const btn = document.getElementById('playBtn');
  if (!btn) return;
  if (playing) {
    btn.innerHTML = '⏸ หยุดวิดีโอและหยุดจับเวลาชั่วคราว';
    btn.className = 'btn btn-pause';
  } else {
    btn.innerHTML = '▶ เล่นวิดีโอและเริ่มจับเวลา';
    btn.className = 'btn btn-play';
  }
}

function updateTimerDisplay(sec) {
  const curMin = String(Math.floor(sec / 60)).padStart(2, '0');
  const curSec = String(Math.floor(sec % 60)).padStart(2, '0');
  set('timeCurrent', `${curMin}:${curSec}`);
  set('lockedCur',    `${curMin}:${curSec}`);

  const pct = Math.min(100, Math.round((sec / lessonDuration) * 100));
  const bar = document.getElementById('videoBar');
  if (bar) bar.style.width = `${pct}%`;

  const pill = document.getElementById('timerPill');
  if (pill) {
    if (sec >= lessonDuration) {
      pill.className = 'timer-pill passed';
      pill.innerText = '✅ เรียนครบเวลาขั้นต่ำแล้ว';
    } else {
      pill.className = 'timer-pill pending';
      pill.innerText = '⏳ ยังไม่ถึงเวลาที่กำหนด';
    }
  }
}

function updatePostLockUI(unlocked) {
  const lockedNotice = document.getElementById('postLocked');
  const activeArea   = document.getElementById('postActive');
  const postTabText  = document.getElementById('postTabText');

  if (unlocked) {
    if (lockedNotice) lockedNotice.classList.add('hidden');
    if (activeArea)   activeArea.classList.remove('hidden');
    if (postTabText)  postTabText.innerText = 'หลังเรียน 🔓';
  } else {
    if (lockedNotice) lockedNotice.classList.remove('hidden');
    if (activeArea)   activeArea.classList.add('hidden');
    if (postTabText)  postTabText.innerText = 'หลังเรียน 🔒';
  }
}

async function sendHeartbeat(sec) {
  if (!currentLesson) return null;
  return await fetchJSON(`/api/lessons/${currentLesson.id}/progress`, 'POST', {
    current_sec: sec,
    playback_rate: 1.0
  });
}

// ── Quizzes Handling ──
async function loadQuiz(quizType, containerId) {
  const quizId = currentQuizzes[quizType];
  const container = document.getElementById(containerId);
  if (!quizId || !container) {
    if (container) container.innerHTML = '<p style="color:var(--muted);padding:1rem">ยังไม่มีแบบทดสอบในบทเรียนนี้</p>';
    return;
  }

  container.innerHTML = '<p style="color:var(--muted);padding:1rem">กำลังโหลดแบบทดสอบ...</p>';
  const data = await fetchJSON(`/api/quizzes/${quizId}`);
  if (!data || !data.success) {
    container.innerHTML = '<p style="color:var(--danger);padding:1rem">ไม่สามารถโหลดข้อสอบได้</p>';
    return;
  }

  renderQuizQuestions(data.quiz, containerId, quizType);
}

function renderQuizQuestions(quiz, containerId, quizType) {
  const container = document.getElementById(containerId);
  if (!quiz.questions || quiz.questions.length === 0) {
    container.innerHTML = '<p style="color:var(--muted);padding:1rem">ไม่มีข้อสอบในชุดนี้</p>';
    return;
  }

  const totalQ = quiz.questions.length;
  let html = `<form id="form_${quizType}" onsubmit="submitQuiz(event, ${quiz.id}, '${quizType}', '${containerId}')">`;
  html += `<div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:0.75rem; font-size:0.82rem; color:var(--muted);">
    <span>📋 แบบทดสอบทั้งหมด <strong>${totalQ}</strong> ข้อ</span>
    <span>เลือกคำตอบให้ครบทุกข้อ</span>
  </div>`;

  quiz.questions.forEach((q, idx) => {
    html += `
      <div class="q-card">
        <div class="q-title">ข้อที่ ${idx + 1} / ${totalQ}: ${escapeHtml(q.question_text)}</div>
        <div class="opt-list">
          ${q.options.map(opt => `
            <label class="opt-label">
              <input type="radio" name="q_${q.id}" value="${opt.id}" required>
              <span>${escapeHtml(opt.option_text)}</span>
            </label>
          `).join('')}
        </div>
      </div>
    `;
  });

  html += `
    <div style="margin-top:1.25rem; padding-bottom:0.75rem;">
      <button type="submit" class="btn btn-primary" style="width:100%; padding:0.8rem; font-size:0.98rem;">
        ✅ ส่งคำตอบและบันทึกผล
      </button>
    </div>
  </form>`;

  container.innerHTML = html;
}

async function submitQuiz(e, quizId, quizType, containerId) {
  e.preventDefault();
  const form = document.getElementById(`form_${quizType}`);
  const formData = new FormData(form);
  const answers = {};

  formData.forEach((value, key) => {
    if (key.startsWith('q_')) {
      const qId = parseInt(key.replace('q_', ''), 10);
      answers[qId] = parseInt(value, 10);
    }
  });

  const res = await fetchJSON(`/api/quizzes/${quizId}/submit`, 'POST', { answers });
  if (!res || !res.success) {
    alert('❌ ไม่สามารถส่งข้อสอบได้: ' + (res ? res.message : 'server error'));
    return;
  }

  const container = document.getElementById(containerId);
  const pass = res.is_passed;
  const score = res.score;
  const total = res.total_score;
  const pct = total > 0 ? Math.round((score / total) * 100) : 0;

  // Reload current dashboard first so certificate data is refreshed
  if (currentUser) {
    if (currentUser.role === 'creator') await loadCreatorDashboard();
    else if (currentUser.role === 'manager') await loadManagerDashboard();
    else await loadLearnerDashboard();
  }

  const isLessonDone = pass && (quizType === 'lesson_post_test' || (res.lesson_progress && res.lesson_progress.is_completed));
  const lessonIdForCert = currentLesson ? currentLesson.id : 0;

  container.innerHTML = `
    <div class="result-box ${pass ? 'pass' : 'fail'}" style="text-align:center; padding:2rem; background:var(--card); border-radius:var(--r); border:1px solid ${pass ? 'var(--success)' : 'var(--danger)'}">
      <div style="font-size:3rem; margin-bottom:0.5rem">${pass ? '🎉' : '❌'}</div>
      <h3 style="color:${pass ? 'var(--success)' : 'var(--danger)'}; margin-bottom:0.5rem">
        ${pass ? 'คุณสอบผ่านเกณฑ์แล้ว!' : 'ยังไม่ผ่านเกณฑ์ 70%'}
      </h3>
      <p style="font-size:1.2rem; font-weight:600; margin-bottom:1rem">คะแนนของคุณ: ${score} / ${total} (${pct}%)</p>
      <p style="font-size:0.85rem; color:var(--muted); margin-bottom:1.5rem">${res.message}</p>
      ${isLessonDone && lessonIdForCert ? `
        <div style="margin-top:1rem; padding:1rem; background:rgba(234,179,8,0.12); border:1px solid rgba(234,179,8,0.4); border-radius:var(--r);">
          <div style="color:#fef08a; font-weight:600; margin-bottom:0.65rem;">🏅 คุณได้รับวุฒิบัตร Soulbound Token (SBT) ประจำบทเรียนนี้แล้ว!</div>
          <div style="display:flex; justify-content:center; gap:0.65rem; flex-wrap:wrap;">
            <button type="button" class="btn btn-wallet" style="border-color:#eab308; color:#fef08a;" onclick="printLessonCertificate(${lessonIdForCert})">🖨️ พิมพ์วุฒิบัตร</button>
            <button type="button" class="btn btn-primary" onclick="emailLessonCertificate(${lessonIdForCert})">📧 ส่งเข้าเมล</button>
          </div>
        </div>
      ` : ''}
      ${!pass ? `<button class="btn btn-primary" onclick="loadQuiz('${quizType}', '${containerId}')">🔄 ลองทำใหม่อีกครั้ง</button>` : ''}
    </div>
  `;
}

// ── Transaction list rendering ──
function renderTxList(records) {
  const wrap = document.getElementById('txList');
  if (!wrap) return;
  if (!records || records.length === 0) {
    wrap.innerHTML = '<div class="tx-empty">ยังไม่มีประวัติ — เริ่มทำแบบทดสอบเพื่อบันทึกผลลงบล็อกเชน</div>';
    return;
  }
  wrap.innerHTML = records.slice(0, 5).map(r => {
    const tx = r.on_chain_tx_hash || '';
    const pct = r.total_score > 0 ? Math.round((r.score / r.total_score) * 100) : 0;
    const txShort = tx ? `${tx.slice(0, 8)}...${tx.slice(-6)}` : 'simulated';
    const href = tx ? `https://sepolia.etherscan.io/tx/${tx}` : '#';
    return `
      <div class="tx-row">
        <div class="tx-info">แบบทดสอบ #${r.quiz_id}: ${r.score}/${r.total_score} (${pct}%) ${r.is_passed ? '✅ ผ่าน' : '❌ ไม่ผ่าน'}</div>
        <div class="tx-hash">Tx: <a class="tx-link" href="${href}" target="_blank" rel="noopener">${txShort}</a></div>
      </div>
    `;
  }).join('');
}

// ============================================================
//  CREATOR: COURSE METADATA & LESSON BUILDER
// ============================================================
function openEditCourseModal() {
  if (!currentCourse) return;
  document.getElementById('editCreatorName').value = (currentCourse.creator && currentCourse.creator.name) ? currentCourse.creator.name : (currentUser ? currentUser.name : '');
  document.getElementById('editCourseTitle').value = currentCourse.title || '';
  document.getElementById('editCourseSummary').value = currentCourse.summary || currentCourse.description || '';
  document.getElementById('editCourseThumbnail').value = currentCourse.thumbnail_url || '';
  document.getElementById('editCourseModal').classList.remove('hidden');
}

function closeEditCourseModal() {
  document.getElementById('editCourseModal').classList.add('hidden');
}

async function saveCourseDetails(e) {
  e.preventDefault();
  if (!currentCourse) return;

  const btn = document.getElementById('saveCourseBtn');
  btn.disabled = true;
  btn.innerText = '⏳ กำลังบันทึก...';

  try {
    const payload = {
      creator_name: document.getElementById('editCreatorName').value.trim(),
      title: document.getElementById('editCourseTitle').value.trim(),
      summary: document.getElementById('editCourseSummary').value.trim(),
      thumbnail_url: document.getElementById('editCourseThumbnail').value.trim()
    };

    const res = await fetchJSON(`/api/courses/${currentCourse.id}`, 'PUT', payload);
    if (res && res.success) {
      alert('🎉 บันทึกการเปลี่ยนแปลงข้อมูลหลักสูตรเรียบร้อยแล้ว!');
      closeEditCourseModal();
      await loadCreatorDashboard();
    } else {
      alert('❌ ไม่สามารถบันทึกได้: ' + (res ? res.message : 'server error'));
    }
  } catch (err) {
    alert('❌ ข้อผิดพลาด: ' + err.message);
  } finally {
    btn.disabled = false;
    btn.innerText = '💾 บันทึกการเปลี่ยนแปลง';
  }
}

// ─── Create Lesson Builder ───
let preQuestions = [];
let postQuestions = [];

function openCreateLessonModal() {
  document.getElementById('newCreatorName').value = currentUser ? currentUser.name : '';
  document.getElementById('newSortOrder').value = allCreatorLessons.length + 1;
  document.getElementById('newOpenPeriod').value = 'เปิดตลอดเวลา';
  const newHoursEl = document.getElementById('newStudyHours');
  if (newHoursEl) newHoursEl.value = 1;
  renderLessonSignatoryCheckboxes('newLessonSignatoriesBox', 'newSigCheck', '');

  if (preQuestions.length === 0) {
    preQuestions = [{
      text: 'ข้อใดคือวัตถุประสงค์หลักของบทเรียนนี้?',
      options: ['เพิ่มประสิทธิภาพและความปลอดภัย', 'ลดความโปร่งใส', 'เพิ่มค่าธรรมเนียม', 'ยกเลิกการตรวจสอบ'],
      correct: 0
    }];
  }

  if (postQuestions.length === 0) {
    postQuestions = [{
      text: 'หลังจากเรียนแล้ว ข้อใดเป็นแนวทางปฏิบัติที่ถูกต้อง?',
      options: ['ปฏิบัติตามมาตรฐานที่กำหนด', 'ข้ามขั้นตอนการทดสอบ', 'เปิดเผย Private Key', 'ไม่บันทึกผล'],
      correct: 0
    }];
  }

  renderQuestionBuilders();
  toggleCreatePreTestSection();
  toggleCreatePostTestSection();
  document.getElementById('createLessonModal').classList.remove('hidden');
}

function closeCreateLessonModal() {
  document.getElementById('createLessonModal').classList.add('hidden');
}

function addQuestionField(type) {
  const arr = (type === 'pre') ? preQuestions : postQuestions;
  arr.push({ text: '', options: ['', '', '', ''], correct: 0 });
  renderQuestionBuilders();
}

function removeQuestionField(type, idx) {
  const arr = (type === 'pre') ? preQuestions : postQuestions;
  if (arr.length <= 1) {
    alert('ควรมีข้อสอบอย่างน้อย 1 ข้อ');
    return;
  }
  arr.splice(idx, 1);
  renderQuestionBuilders();
}

function renderQuestionBuilders() {
  renderQuestionList('pre', preQuestions, 'preQuestionsContainer');
  renderQuestionList('post', postQuestions, 'postQuestionsContainer');
}

function renderQuestionList(type, questions, containerId) {
  const container = document.getElementById(containerId);
  if (!container) return;

  container.innerHTML = questions.map((q, qIdx) => `
    <div class="question-card" style="background:var(--card); border:1px solid var(--border); border-radius:var(--r); padding:1rem; margin-bottom:1rem;">
      <div style="display:flex; justify-content:space-between; margin-bottom:0.5rem;">
        <span style="font-weight:600; font-size:0.85rem">คำถามข้อที่ ${qIdx + 1} (${type === 'pre' ? 'ก่อนเรียน' : 'หลังเรียน'}):</span>
        <button type="button" class="btn btn-sm" style="color:#f87171" onclick="removeQuestionField('${type}', ${qIdx})">🗑️ ลบ</button>
      </div>
      <input type="text" class="form-input" placeholder="พิมพ์คำถาม..." value="${escapeHtml(q.text)}"
             onchange="${type}Questions[${qIdx}].text = this.value" required style="margin-bottom:0.75rem;">
      <label class="form-label" style="font-size:0.75rem">ตัวเลือกคำตอบ (เลือกปุ่ม ⭕ เพื่อเป็นคำตอบที่ถูกต้อง):</label>
      ${q.options.map((opt, optIdx) => `
        <div style="display:flex; align-items:center; gap:0.5rem; margin-bottom:0.35rem">
          <input type="radio" name="${type}_q_${qIdx}_correct" value="${optIdx}" ${q.correct === optIdx ? 'checked' : ''}
                 onchange="${type}Questions[${qIdx}].correct = ${optIdx}">
          <input type="text" class="form-input" placeholder="ตัวเลือกที่ ${optIdx + 1}" value="${escapeHtml(opt)}"
                 onchange="${type}Questions[${qIdx}].options[${optIdx}] = this.value" required>
        </div>
      `).join('')}
    </div>
  `).join('');
}

function toggleCreatePreTestSection() {
  const hasPre = !!(document.getElementById('newHasPreTest') ? document.getElementById('newHasPreTest').checked : true);
  const cont = document.getElementById('preQuestionsContainer');
  const addBtn = document.getElementById('btnAddPreQuestion');
  if (cont) {
    cont.classList.toggle('hidden', !hasPre);
    cont.querySelectorAll('input').forEach(inp => { inp.disabled = !hasPre; });
  }
  if (addBtn) addBtn.classList.toggle('hidden', !hasPre);
}

function toggleCreatePostTestSection() {
  const hasPost = !!(document.getElementById('newHasPostTest') ? document.getElementById('newHasPostTest').checked : true);
  const cont = document.getElementById('postQuestionsContainer');
  const addBtn = document.getElementById('btnAddPostQuestion');
  if (cont) {
    cont.classList.toggle('hidden', !hasPost);
    cont.querySelectorAll('input').forEach(inp => { inp.disabled = !hasPost; });
  }
  if (addBtn) addBtn.classList.toggle('hidden', !hasPost);
}

async function submitNewLesson(e) {
  e.preventDefault();
  const btn = document.getElementById('submitLessonBtn');
  btn.disabled = true;
  btn.innerText = '⏳ กำลังบันทึก...';

  try {
    const title = document.getElementById('newLessonTitle').value.trim();
    const hasPreTest = !!(document.getElementById('newHasPreTest') ? document.getElementById('newHasPreTest').checked : true);
    const hasPostTest = !!(document.getElementById('newHasPostTest') ? document.getElementById('newHasPostTest').checked : true);
    const passScorePct = parseFloat(document.getElementById('newPassScorePct')?.value) || 70;
    const quizzesPayload = [];
    if (hasPreTest && preQuestions.length > 0) {
      quizzesPayload.push({
        title: `${title}: แบบทดสอบก่อนเรียน`,
        type: 'lesson_pre_test',
        questions: preQuestions.map(q => ({
          question_text: q.text,
          options: q.options.map((opt, idx) => ({ option_text: opt, is_correct: (q.correct === idx) }))
        }))
      });
    }
    if (hasPostTest && postQuestions.length > 0) {
      quizzesPayload.push({
        title: `${title}: แบบทดสอบหลังเรียน`,
        type: 'lesson_post_test',
        questions: postQuestions.map(q => ({
          question_text: q.text,
          options: q.options.map((opt, idx) => ({ option_text: opt, is_correct: (q.correct === idx) }))
        }))
      });
    }

    const payload = {
      course_id: currentCourse ? currentCourse.id : 0,
      creator_name: document.getElementById('newCreatorName').value.trim(),
      title: title,
      summary: document.getElementById('newLessonSummary').value.trim(),
      description: document.getElementById('newLessonSummary').value.trim(),
      youtube_url: document.getElementById('newYoutubeURL').value.trim(),
      duration_sec: parseFloat(document.getElementById('newDurationSec').value) || 60,
      study_hours: document.getElementById('newStudyHours') ? (parseFloat(document.getElementById('newStudyHours').value) || 1) : 1,
      sort_order: parseInt(document.getElementById('newSortOrder').value, 10) || 1,
      thumbnail_url: document.getElementById('newThumbnailURL').value.trim() || 'https://images.unsplash.com/photo-1622979135225-d2ba269bc1df?q=80&w=400',
      open_period: document.getElementById('newOpenPeriod').value.trim() || 'เปิดตลอดเวลา',
      pass_score_pct: passScorePct,
      disable_pre_test: !hasPreTest,
      disable_post_test: !hasPostTest,
      signatory_ids: collectSelectedSignatoryIDs('newSigCheck'),
      quizzes: quizzesPayload
    };

    const res = await fetchJSON('/api/lessons', 'POST', payload);
    if (res && res.success) {
      alert(`🎉 สำเร็จ! แทรกบทเรียน "${title}" เข้าสู่ระบบเรียบร้อยแล้ว`);
      closeCreateLessonModal();
      document.getElementById('createLessonForm').reset();
      preQuestions = [];
      postQuestions = [];
      await loadCreatorDashboard();
    } else {
      alert('❌ ไม่สามารถสร้างบทเรียนได้: ' + (res ? res.message : 'server error'));
    }
  } catch (err) {
    alert('❌ ข้อผิดพลาด: ' + err.message);
  } finally {
    btn.disabled = false;
    btn.innerText = '💾 บันทึกและแทรกบทเรียนเข้าสู่ระบบ';
  }
}

// ============================================================
//  UTILITIES & HELPERS
// ============================================================
function set(id, text) {
  const el = document.getElementById(id);
  if (el) el.innerText = text;
}

function capitalize(s) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

function escapeHtml(str) {
  if (!str) return '';
  return String(str).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function extractYoutubeId(url) {
  if (!url) return '';
  const match = url.match(/(?:youtu\.be\/|youtube\.com\/(?:embed\/|v\/|watch\?v=|watch\?.+&v=))([\w-]{11})/);
  return match ? match[1] : '';
}

async function fetchJSON(url, method = 'GET', body = null) {
  try {
    const opts = {
      method,
      headers: {
        'Content-Type': 'application/json',
        'X-User-ID': String(currentUserID)
      }
    };
    if (body) opts.body = JSON.stringify(body);
    const res  = await fetch(url, opts);
    const text = await res.text();
    try {
      return JSON.parse(text);
    } catch {
      console.error('Non-JSON response from', url, ':', text.slice(0, 200));
      return null;
    }
  } catch (err) {
    console.error('Fetch error:', url, err.message);
    return null;
  }
}

// ============================================================
//  MULTI-MACHINE SYNC & LAN SHARING (เชื่อมต่อและซิงค์ข้ามเครื่อง)
// ============================================================
let syncModalPollTimer = null;

async function openSyncModal() {
  const modal = document.getElementById('syncModal');
  if (modal) modal.classList.remove('hidden');

  const statusBox = document.getElementById('syncResultStatusBox');
  if (statusBox) statusBox.classList.add('hidden');

  await loadSyncInfo(true);

  if (syncModalPollTimer) clearInterval(syncModalPollTimer);
  syncModalPollTimer = setInterval(() => {
    const m = document.getElementById('syncModal');
    if (m && !m.classList.contains('hidden')) {
      loadSyncInfo(false);
    }
  }, 5000);
}

function closeSyncModal() {
  const modal = document.getElementById('syncModal');
  if (modal) modal.classList.add('hidden');
  if (syncModalPollTimer) {
    clearInterval(syncModalPollTimer);
    syncModalPollTimer = null;
  }
}

async function loadSyncInfo(populateInputs = false) {
  const res = await fetchJSON('/api/sync/info');
  if (!res || !res.success) return;

  const hostLabel = document.getElementById('syncHostnameLabel');
  if (hostLabel) hostLabel.innerText = res.hostname || 'เครื่องหลัก';

  const onlineCount = res.online_count || 1;
  const onlineBadge = document.getElementById('syncOnlineCountBadge');
  if (onlineBadge) {
    onlineBadge.innerText = `🟢 เชื่อมต่ออยู่ ${onlineCount} เครื่อง`;
  }
  const navSyncBtn = document.getElementById('btnSyncMenu');
  if (navSyncBtn) {
    navSyncBtn.innerText = onlineCount > 1 ? `🔄 ซิงค์เครื่อง (${onlineCount})` : `🔄 ซิงค์เครื่อง`;
  }

  const isGod = !!res.is_god_therunrock;
  const gasGroup = document.getElementById('syncGasUrlGroup');
  const btnCopyGas = document.getElementById('btnCopyGasScript');
  const masterOverrideGroup = document.getElementById('syncMasterOverrideGroup');
  if (gasGroup) gasGroup.classList.toggle('hidden', !isGod);
  if (btnCopyGas) btnCopyGas.classList.toggle('hidden', !isGod);
  if (masterOverrideGroup) masterOverrideGroup.classList.toggle('hidden', !isGod);

  const gasDot = document.getElementById('syncGasStatusDot');
  const gasLabel = document.getElementById('syncGasStatusLabel');
  if (gasDot && gasLabel) {
    const st = res.gas_status || 'standby';
    const colorMap = {
      connected: { bg: '#10b981', glow: '0 0 7px rgba(16,185,129,0.65)' },
      standby:   { bg: '#f59e0b', glow: '0 0 7px rgba(245,158,11,0.65)' },
      error:     { bg: '#ef4444', glow: '0 0 7px rgba(239,68,68,0.65)' }
    };
    const c = colorMap[st] || colorMap.standby;
    gasDot.style.background = c.bg;
    gasDot.style.boxShadow = c.glow;
    gasLabel.innerText = `สถานะการเชื่อมต่อ GAS: ${res.gas_status_text || 'รอเชื่อมต่อ'}`;
  }

  const transportBanner = document.getElementById('syncTransportBanner');
  if (transportBanner) {
    transportBanner.innerHTML = `
      <div>📡 <strong>โหมดเชื่อมต่อ:</strong> <code>${escapeHtml(res.sync_transport || 'LAN/WiFi Auto')}</code> | 👑 <strong>Master:</strong> <code>${escapeHtml(res.elected_master_id || res.node_id || '-')}</code> | ⚡ <strong>Stability Score:</strong> ${res.local_stability_score ?? '-'}</div>
    `;
  }

  if (populateInputs && res.config) {
    const peerInput = document.getElementById('syncPeerUrlInput');
    if (peerInput && isGod) {
      peerInput.value = res.config.gas_url || '';
    }
    const autoCheck = document.getElementById('syncAutoEnabledCheckbox');
    if (autoCheck) {
      autoCheck.checked = res.config.auto_sync_enabled !== false;
    }
    const timeoutInput = document.getElementById('syncTimeoutSecInput');
    if (timeoutInput && res.config.timeout_sec) {
      timeoutInput.value = res.config.timeout_sec;
    }
  }

  // Populate IP & Master selector dropdowns
  const ipSelect = document.getElementById('syncPreferredIpSelect');
  const masterSelect = document.getElementById('syncPreferredMasterSelect');
  const nodes = res.active_nodes || [];
  if (ipSelect) {
    const currentVal = (res.config && res.config.preferred_master_ip) ? res.config.preferred_master_ip : 'auto';
    const ipOptions = ['<option value="auto">⚡ อัตโนมัติ (ทดสอบและเลือก IP ที่เร็วที่สุด)</option>'];
    const seenIPs = new Set();
    nodes.forEach(n => {
      const urls = (n.lan_urls && n.lan_urls.length) ? n.lan_urls : (n.lan_url ? [n.lan_url] : []);
      urls.forEach(u => {
        if (u && !seenIPs.has(u)) {
          seenIPs.add(u);
          const tag = n.is_master ? '👑 Master' : (n.hostname || n.node_id);
          ipOptions.push(`<option value="${escapeHtml(u)}">${escapeHtml(u)} (${escapeHtml(tag)})</option>`);
        }
      });
    });
    ipSelect.innerHTML = ipOptions.join('');
    ipSelect.value = currentVal || 'auto';
  }
  if (masterSelect && isGod) {
    const curMaster = (res.config && res.config.preferred_master_id) ? res.config.preferred_master_id : 'auto';
    const mOpts = ['<option value="auto">⚡ อัตโนมัติ (เลือกเครื่องที่เสถียรภาพสูงสุด)</option>'];
    nodes.forEach(n => {
      mOpts.push(`<option value="${escapeHtml(n.node_id)}">${escapeHtml(n.hostname || n.node_id)} (Score: ${n.stability_score ?? 0})</option>`);
    });
    masterSelect.innerHTML = mOpts.join('');
    masterSelect.value = curMaster || 'auto';
  }

  const lastSyncLabel = document.getElementById('syncLastSyncLabel');
  if (lastSyncLabel) {
    if (res.last_sync_at) {
      lastSyncLabel.innerText = `${res.last_sync_status || 'ซิงค์แล้ว'} (${res.last_sync_at})`;
    } else {
      lastSyncLabel.innerText = 'พร้อมซิงค์อัตโนมัติ';
    }
  }

  const countsBox = document.getElementById('syncLocalCountsBox');
  if (countsBox && res.counts) {
    const c = res.counts;
    countsBox.innerHTML = `
      <span class="badge-pill" style="background:rgba(14,165,233,0.12); color:var(--primary); padding:0.25rem 0.65rem; border-radius:999px; font-weight:600;">👥 สมาชิก: ${c.users || 0} คน</span>
      <span class="badge-pill" style="background:rgba(16,185,129,0.12); color:#059669; padding:0.25rem 0.65rem; border-radius:999px; font-weight:600;">📚 บทเรียน: ${c.lessons || 0} บท</span>
      <span class="badge-pill" style="background:rgba(245,158,11,0.14); color:#d97706; padding:0.25rem 0.65rem; border-radius:999px; font-weight:600;">📌 มอบหมาย: ${c.assignments || 0} รายการ</span>
      <span class="badge-pill" style="background:rgba(99,102,241,0.12); color:#4f46e5; padding:0.25rem 0.65rem; border-radius:999px; font-weight:600;">📈 การเรียนรู้: ${c.progress || 0} รายการ</span>
      <span class="badge-pill" style="background:rgba(236,72,153,0.12); color:#db2777; padding:0.25rem 0.65rem; border-radius:999px; font-weight:600;">🏅 ใบประกาศ: ${c.certificates || 0} ใบ</span>
    `;
  }

  // วิธีที่ 1: แสดง QR Code และลิงก์ LAN
  const lanList = document.getElementById('syncLanUrlList');
  if (lanList) {
    const urls = (res.lan_urls && res.lan_urls.length > 0) ? res.lan_urls : [window.location.origin];
    lanList.innerHTML = urls.map(u => {
      const qrSrc = `/api/sync/qr?url=${encodeURIComponent(u)}`;
      return `
        <div style="display:flex; align-items:center; gap:1rem; flex-wrap:wrap; background:var(--bg); border:1px solid var(--border); padding:0.75rem 1rem; border-radius:10px;">
          <div style="background:#ffffff; padding:6px; border-radius:8px; border:1px solid #cbd5e1; display:flex; align-items:center; justify-content:center; box-shadow:0 2px 6px rgba(0,0,0,0.06);">
            <img src="${qrSrc}" alt="QR Code ${escapeHtml(u)}" style="width:128px; height:128px; display:block;" />
          </div>
          <div style="flex:1; min-width:200px; display:flex; flex-direction:column; gap:0.45rem;">
            <div style="font-size:0.78rem; color:var(--muted); font-weight:600;">📱 สแกน QR Code ด้วยมือถือ / iPad หรือเปิดลิงก์:</div>
            <code style="font-size:0.95rem; font-weight:700; color:var(--primary); word-break:break-all;">${escapeHtml(u)}</code>
            <div style="display:flex; gap:0.5rem; margin-top:0.2rem;">
              <button type="button" class="btn btn-sm btn-wallet" onclick="copySyncLanUrl('${escapeHtml(u)}')">📋 คัดลอกลิงก์</button>
            </div>
          </div>
        </div>
      `;
    }).join('');
  }

  // วิธีที่ 2: แสดงรายการเครื่องที่เชื่อมต่ออยู่
  const nodesListEl = document.getElementById('syncActiveNodesList');
  if (nodesListEl) {
    const nodes = res.active_nodes || [];
    if (nodes.length === 0) {
      nodesListEl.innerHTML = `<div style="color:var(--muted);">ไม่มีข้อมูลเครื่องที่เชื่อมต่อ</div>`;
    } else {
      nodesListEl.innerHTML = nodes.map(n => `
        <div style="display:flex; align-items:center; justify-content:space-between; flex-wrap:wrap; gap:0.4rem; background:var(--bg); border:1px solid var(--border); padding:0.45rem 0.75rem; border-radius:8px;">
          <div style="display:flex; align-items:center; gap:0.45rem;">
            <span style="display:inline-block; width:8px; height:8px; border-radius:50%; background:#10b981;"></span>
            <strong style="color:var(--text);">${escapeHtml(n.hostname || n.node_id)}</strong>
            ${n.is_master ? '<span style="font-size:0.7rem; background:rgba(245,158,11,0.18); color:#d97706; padding:0.1rem 0.45rem; border-radius:999px; font-weight:700;">👑 Master</span>' : '<span style="font-size:0.7rem; background:rgba(100,116,139,0.15); color:var(--muted); padding:0.1rem 0.45rem; border-radius:999px; font-weight:600;">Peer</span>'}
            ${n.is_self ? '<span style="font-size:0.7rem; background:rgba(14,165,233,0.15); color:var(--primary); padding:0.1rem 0.45rem; border-radius:999px; font-weight:700;">เครื่องนี้</span>' : ''}
            <code style="font-size:0.78rem; color:var(--primary);">${escapeHtml(n.lan_url || '')}</code>
          </div>
          <div style="font-size:0.74rem; color:var(--muted);">
            Score: ${n.stability_score ?? 0}${n.latency_ms ? ` (${n.latency_ms}ms)` : ''} | สมาชิก ${n.users_count || 0} | บทเรียน ${n.lessons_count || 0} | ใบประกาศ ${n.certs_count || 0}
          </div>
        </div>
      `).join('');
    }
  }
}

function copySyncLanUrl(url) {
  if (navigator.clipboard) {
    navigator.clipboard.writeText(url);
  }
  showSyncStatus(`✅ คัดลอกลิงก์ ${url} เรียบร้อยแล้ว นำไปเปิดในเบราว์เซอร์ของอีกเครื่องได้ทันที`, true);
}

async function copyGasScriptCode() {
  try {
    const res = await fetchJSON('/api/sync/gas-script');
    if (res && res.success && res.script) {
      if (navigator.clipboard) {
        await navigator.clipboard.writeText(res.script);
      }
      showSyncStatus(
        `✅ <strong>คัดลอกโค้ด Google Apps Script ลงคลิปบอร์ดแล้ว!</strong><br>` +
        `1. เปิด Google Sheet ใหม่ -> เมนู <strong>ส่วนขยาย (Extensions) > Apps Script</strong><br>` +
        `2. วางโค้ดลงไป กด <strong>Deploy > New deployment > Web app</strong> (เลือกเข้าถึงได้: <em>ทุกคน / Anyone</em>)<br>` +
        `3. คัดลอก URL ที่ได้มาวางในช่องด้านบนแล้วกด <strong>"บันทึก & ซิงค์ข้อมูลทันที"</strong>`,
        true
      );
    } else {
      showSyncStatus('❌ ไม่สามารถโหลดโค้ด Google Apps Script ได้', false);
    }
  } catch (err) {
    showSyncStatus('❌ เกิดข้อผิดพลาด: ' + escapeHtml(err.message), false);
  }
}

function showSyncStatus(msg, isSuccess = true) {
  const box = document.getElementById('syncResultStatusBox');
  if (!box) return;
  box.classList.remove('hidden');
  box.style.background = isSuccess ? 'rgba(16,185,129,0.14)' : 'rgba(239,68,68,0.14)';
  box.style.border = isSuccess ? '1px solid rgba(16,185,129,0.4)' : '1px solid rgba(239,68,68,0.4)';
  box.style.color = isSuccess ? '#059669' : '#dc2626';
  box.innerHTML = msg;
}

async function submitPeerSync() {
  const input = document.getElementById('syncPeerUrlInput');
  const autoCheck = document.getElementById('syncAutoEnabledCheckbox');
  const timeoutInput = document.getElementById('syncTimeoutSecInput');
  const ipSelect = document.getElementById('syncPreferredIpSelect');
  const masterSelect = document.getElementById('syncPreferredMasterSelect');
  const btn = document.getElementById('btnPeerSyncSubmit');

  const peerUrl = input ? input.value.trim() : '';
  const autoEnabled = autoCheck ? autoCheck.checked : true;
  const timeoutSec = timeoutInput ? (parseInt(timeoutInput.value, 10) || 180) : 180;
  const preferredIP = ipSelect ? ipSelect.value : 'auto';
  const preferredMaster = masterSelect ? masterSelect.value : 'auto';

  if (btn) {
    btn.disabled = true;
    btn.innerText = '⏳ กำลังทดสอบความเร็ว IP และซิงค์ข้อมูล...';
  }

  try {
    // บันทึกค่า Auto-Sync Config และสั่งซิงค์ทันที (ไม่บังคับกรอก GAS URL สำหรับผู้ใช้ทั่วไป)
    const res = await fetchJSON('/api/sync/config', 'POST', {
      gas_url: peerUrl,
      auto_sync_enabled: autoEnabled,
      interval_sec: 30,
      timeout_sec: timeoutSec,
      preferred_master_ip: preferredIP === 'auto' ? '' : preferredIP,
      preferred_master_id: preferredMaster === 'auto' ? '' : preferredMaster
    });
    if (res && res.success) {
      const st = res.pulled_stats || {};
      const onlineCnt = res.online_count || 1;
      const detail = `ออนไลน์ ${onlineCnt} เครื่อง | สมาชิก +${st.users_added || 0} | บทเรียน +${st.lessons_added || 0} | การเรียนรู้ +${st.progress_added || 0} (อัปเดต ${st.progress_updated || 0}) | ใบประกาศ +${st.certs_added || 0}`;
      showSyncStatus(`<strong>${escapeHtml(res.message || 'ซิงค์ข้อมูลสำเร็จ')}</strong><br><span style="font-size:0.79rem;">${detail}</span>`, true);
      await loadSyncInfo(false);
      await refreshCurrentPortalAfterSync();
    } else {
      showSyncStatus('❌ ' + (res && res.error ? escapeHtml(res.error) : 'ไม่สามารถเชื่อมต่อกับปลายทางได้'), false);
    }
  } catch (err) {
    showSyncStatus('❌ เกิดข้อผิดพลาด: ' + escapeHtml(err.message), false);
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.innerText = '🔄 บันทึก & ซิงค์ข้อมูลทันที';
    }
  }
}

async function exportSyncSnapshotFile() {
  try {
    const snap = await fetchJSON('/api/sync/export');
    if (!snap || !snap.version) {
      showSyncStatus('❌ ไม่สามารถส่งออกข้อมูลได้', false);
      return;
    }
    const blob = new Blob([JSON.stringify(snap, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    const dateStr = new Date().toISOString().slice(0, 10);
    a.href = url;
    a.download = `LearnChainNurse_Sync_${dateStr}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    showSyncStatus('✅ ส่งออกไฟล์ข้อมูลสำเร็จ! นำไฟล์นี้ไปกด "นำเข้าไฟล์ข้อมูล" ที่อีกเครื่องได้เลย', true);
  } catch (err) {
    showSyncStatus('❌ ส่งออกไฟล์ไม่สำเร็จ: ' + escapeHtml(err.message), false);
  }
}

async function importSyncSnapshotFile(event) {
  const file = event.target.files && event.target.files[0];
  if (!file) return;
  try {
    const text = await file.text();
    const snap = JSON.parse(text);
    const res = await fetchJSON('/api/sync/import', 'POST', snap);
    if (res && res.success) {
      const st = res.stats || {};
      const detail = `สมาชิก +${st.users_added || 0} (อัปเดต ${st.users_updated || 0}) | บทเรียน +${st.lessons_added || 0} | การเรียนรู้ +${st.progress_added || 0} (อัปเดต ${st.progress_updated || 0}) | ใบประกาศ +${st.certs_added || 0}`;
      showSyncStatus(`<strong>✅ ${escapeHtml(res.message)}</strong><br><span style="font-size:0.79rem;">${detail}</span>`, true);
      await loadSyncInfo();
      await refreshCurrentPortalAfterSync();
    } else {
      showSyncStatus('❌ นำเข้าข้อมูลไม่สำเร็จ: ' + (res && res.error ? escapeHtml(res.error) : 'ไฟล์ไม่ถูกต้อง'), false);
    }
  } catch (err) {
    showSyncStatus('❌ ไฟล์ข้อมูลไม่ถูกต้อง: ' + escapeHtml(err.message), false);
  } finally {
    event.target.value = '';
  }
}

async function refreshCurrentPortalAfterSync() {
  if (!isAuthenticated || !currentUserID) return;
  await fetchCurrentUser();
  if (activeRole === 'learner') {
    await loadLearnerDashboard();
  } else if (activeRole === 'creator') {
    await loadCreatorDashboard();
  } else if (activeRole === 'manager') {
    await loadManagerDashboard();
  }
}

// ============================================================
//  MOBILE NAVBAR TOGGLE & COLLAPSIBLE FLOATING CERTS BUTTON
// ============================================================
function toggleMobileNav(forceState) {
  const menu = document.getElementById('navRightMenu');
  const label = document.getElementById('floatingMenuLabel');
  const wrap = document.getElementById('floatingMenuWrap');
  if (!menu) return;
  const isOpen = (typeof forceState === 'boolean')
    ? (forceState ? (menu.classList.add('nav-open'), true) : (menu.classList.remove('nav-open'), false))
    : menu.classList.toggle('nav-open');
  if (label) {
    label.innerText = isOpen ? 'ปิดเมนู' : 'เมนู';
  }
  if (wrap) {
    wrap.classList.toggle('menu-active', isOpen);
  }
  localStorage.setItem('lc_menu_open', isOpen ? 'true' : 'false');
}

function onFloatingMenuClick() {
  const wrap = document.getElementById('floatingMenuWrap');
  if (wrap && wrap.classList.contains('collapsed')) {
    wrap.classList.remove('collapsed');
    localStorage.setItem('lc_menu_collapsed', 'false');
    return;
  }
  toggleMobileNav();
}

function toggleFloatingMenuCollapse(e) {
  if (e) e.stopPropagation();
  const wrap = document.getElementById('floatingMenuWrap');
  if (!wrap) return;
  const isCollapsed = wrap.classList.toggle('collapsed');
  localStorage.setItem('lc_menu_collapsed', isCollapsed ? 'true' : 'false');
}

function initFloatingMenuState() {
  const wrap = document.getElementById('floatingMenuWrap');
  if (wrap && localStorage.getItem('lc_menu_collapsed') === 'true') {
    wrap.classList.add('collapsed');
  }
  if (localStorage.getItem('lc_menu_open') === 'true') {
    toggleMobileNav(true);
  }
}

function initFloatingCertsState() {
  const wrap = document.getElementById('btnLearnerCerts');
  if (!wrap) return;
  const saved = localStorage.getItem('lc_certs_collapsed');
  const shouldCollapse = saved !== null ? (saved === 'true') : (window.innerWidth <= 768);
  if (shouldCollapse) {
    wrap.classList.add('collapsed');
  } else {
    wrap.classList.remove('collapsed');
  }
}

function onFloatingCertsClick() {
  const wrap = document.getElementById('btnLearnerCerts');
  if (!wrap) return;
  if (wrap.classList.contains('collapsed')) {
    wrap.classList.remove('collapsed');
    localStorage.setItem('lc_certs_collapsed', 'false');
    return;
  }
  openMyCertificatesModal();
}

function toggleFloatingCertsCollapse(e) {
  if (e) e.stopPropagation();
  const wrap = document.getElementById('btnLearnerCerts');
  if (!wrap) return;
  const isCollapsed = wrap.classList.toggle('collapsed');
  localStorage.setItem('lc_certs_collapsed', isCollapsed ? 'true' : 'false');
}

function initFloatingThemeState() {
  const wrap = document.getElementById('floatingThemeWrap');
  if (!wrap) return;
  if (localStorage.getItem('lc_theme_collapsed') === 'true') {
    wrap.classList.add('collapsed');
  }
}

function onFloatingThemeIconClick() {
  const wrap = document.getElementById('floatingThemeWrap');
  if (!wrap) return;
  if (wrap.classList.contains('collapsed')) {
    wrap.classList.remove('collapsed');
    localStorage.setItem('lc_theme_collapsed', 'false');
  } else {
    wrap.classList.add('collapsed');
    localStorage.setItem('lc_theme_collapsed', 'true');
  }
}

function toggleFloatingThemeCollapse(e) {
  if (e) e.stopPropagation();
  const wrap = document.getElementById('floatingThemeWrap');
  if (!wrap) return;
  const isCollapsed = wrap.classList.toggle('collapsed');
  localStorage.setItem('lc_theme_collapsed', isCollapsed ? 'true' : 'false');
}

document.addEventListener('DOMContentLoaded', initFloatingCertsState);
document.addEventListener('DOMContentLoaded', initFloatingThemeState);
document.addEventListener('DOMContentLoaded', initFloatingMenuState);

// ============================================================
//  MEMBER MANAGEMENT (จัดการสมาชิก: ปรับลดสิทธิ์ / ลบสมาชิก)
// ============================================================
let allManagedMembers = [];

async function openMemberManagementModal() {
  const modal = document.getElementById('memberManagementModal');
  if (modal) modal.classList.remove('hidden');
  await loadMembersList();
}

function closeMemberManagementModal() {
  const modal = document.getElementById('memberManagementModal');
  if (modal) modal.classList.add('hidden');
}

async function loadMembersList() {
  const listEl = document.getElementById('memberManagementList');
  if (listEl) listEl.innerHTML = '<div style="color:var(--muted); text-align:center; padding:1.5rem;">⏳ กำลังโหลดรายชื่อสมาชิก...</div>';
  const res = await fetchJSON('/api/manager/members');
  if (!res || !res.success) {
    if (listEl) listEl.innerHTML = `<div style="color:var(--danger); text-align:center; padding:1.5rem;">❌ ไม่สามารถโหลดข้อมูลสมาชิกได้: ${escapeHtml((res && res.message) || 'ไม่มีสิทธิ์เข้าถึง')}</div>`;
    return;
  }
  allManagedMembers = res.members || [];
  renderMemberManagementList();
}

function renderMemberManagementList() {
  const listEl = document.getElementById('memberManagementList');
  const countEl = document.getElementById('memberCountSummary');
  if (!listEl) return;

  const q = (document.getElementById('memberSearchInput')?.value || '').trim().toLowerCase();
  const wardF = document.getElementById('memberWardFilter')?.value || '';
  const roleF = document.getElementById('memberRoleFilter')?.value || '';

  const filtered = allManagedMembers.filter(m => {
    const matchText = !q ||
      (m.name && m.name.toLowerCase().includes(q)) ||
      (m.email && m.email.toLowerCase().includes(q));
    const matchWard = !wardF || (m.ward === wardF);
    const matchRole = !roleF || (m.highest_role === roleF);
    return matchText && matchWard && matchRole;
  });

  if (countEl) {
    countEl.innerText = `แสดง ${filtered.length} จากทั้งหมด ${allManagedMembers.length} คน`;
  }

  if (filtered.length === 0) {
    listEl.innerHTML = '<div style="color:var(--muted); text-align:center; padding:1.5rem; border:1px dashed var(--border); border-radius:var(--r);">ไม่พบสมาชิกตามเงื่อนไขที่ค้นหา</div>';
    return;
  }

  const roleBadgeMap = {
    learner: { label: '🎓 ผู้เรียน', style: 'background:rgba(14,165,233,0.12); color:var(--primary); border:1px solid rgba(14,165,233,0.3);' },
    creator: { label: '👨‍🏫 ผู้สร้างบทเรียน', style: 'background:rgba(139,92,246,0.12); color:#7c3aed; border:1px solid rgba(139,92,246,0.3);' },
    manager: { label: '👔 ผู้จัดการ', style: 'background:rgba(16,185,129,0.12); color:#059669; border:1px solid rgba(16,185,129,0.35);' }
  };

  listEl.innerHTML = filtered.map((m, idx) => {
    const rb = roleBadgeMap[m.highest_role] || roleBadgeMap.learner;
    const isSelf = currentUser && currentUser.email && m.email && (currentUser.email.toLowerCase() === m.email.toLowerCase());
    const safeEmail = escapeHtml(m.email || '');
    const safeName = escapeHtml(m.name || 'ไม่ระบุชื่อ');

    return `
      <div style="background:var(--card2); border:1px solid var(--border); border-radius:10px; padding:0.75rem 0.9rem; display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:0.65rem;">
        <div style="min-width:200px; flex:1;">
          <div style="display:flex; align-items:center; gap:0.45rem; flex-wrap:wrap; margin-bottom:0.2rem;">
            <strong style="font-size:0.95rem; color:var(--text);">${safeName}</strong>
            <span style="font-size:0.72rem; font-weight:700; padding:0.12rem 0.55rem; border-radius:999px; ${rb.style}">${rb.label}</span>
            ${isSelf ? '<span style="font-size:0.7rem; background:rgba(245,158,11,0.15); color:#d97706; padding:0.1rem 0.45rem; border-radius:999px; font-weight:700;">บัญชีคุณ</span>' : ''}
          </div>
          <div style="font-size:0.78rem; color:var(--muted);">
            📧 ${safeEmail || '-'} &nbsp;•&nbsp; 🏥 หน่วยงาน: <strong>${escapeHtml(m.ward || 'สว.11')}</strong>
          </div>
        </div>

        <div style="display:flex; align-items:center; gap:0.45rem; flex-wrap:wrap;">
          <select id="memberRoleSel_${idx}" class="form-input" style="padding:0.32rem 0.55rem; font-size:0.8rem; width:auto; min-width:140px;" ${isSelf ? 'disabled' : ''}>
            <option value="learner" ${m.highest_role === 'learner' ? 'selected' : ''}>🎓 ผู้เรียน</option>
            <option value="creator" ${m.highest_role === 'creator' ? 'selected' : ''}>👨‍🏫 ผู้สร้างบทเรียน</option>
            <option value="manager" ${m.highest_role === 'manager' ? 'selected' : ''}>👔 ผู้จัดการ</option>
          </select>
          <button type="button" class="btn btn-sm btn-primary" ${isSelf ? 'disabled' : ''} onclick="changeMemberRole('${safeEmail}', ${m.primary_id}, ${idx})">
            💾 บันทึกสิทธิ์
          </button>
          <button type="button" class="btn btn-sm" style="background:rgba(239,68,68,0.12); color:var(--danger); border:1px solid rgba(239,68,68,0.35);" ${isSelf ? 'disabled' : ''} onclick="deleteMemberAccount('${safeEmail}', ${m.primary_id}, '${safeName}')">
            🗑️ ลบ
          </button>
        </div>
      </div>
    `;
  }).join('');
}

async function changeMemberRole(email, primaryId, idx) {
  const sel = document.getElementById(`memberRoleSel_${idx}`);
  const targetRole = sel ? sel.value : 'learner';
  const res = await fetchJSON('/api/manager/members/role', 'PUT', {
    email: email,
    user_id: primaryId,
    target_role: targetRole
  });
  if (res && res.success) {
    alert('✅ ' + res.message);
    await loadMembersList();
    if (activeRole === 'manager') await loadManagerDashboard();
  } else {
    alert('❌ ' + ((res && res.message) || 'ไม่สามารถปรับสิทธิ์ได้'));
  }
}

async function deleteMemberAccount(email, primaryId, name) {
  if (!confirm(`⚠️ ต้องการลบสมาชิก "${name}" (${email}) ออกจากระบบถาวรใช่หรือไม่?\n(ประวัติการเรียนและวุฒิบัตรของสมาชิกท่านนี้จะถูกลบออกด้วย)`)) return;
  const qs = email ? `email=${encodeURIComponent(email)}` : `user_id=${primaryId}`;
  const res = await fetchJSON(`/api/manager/members?${qs}`, 'DELETE');
  if (res && res.success) {
    alert('✅ ' + res.message);
    await loadMembersList();
    if (activeRole === 'manager') await loadManagerDashboard();
  } else {
    alert('❌ ' + ((res && res.message) || 'ไม่สามารถลบสมาชิกได้'));
  }
}

// ============================================================
//  BLOCK LIFECYCLE & ARCHIVE (ระบบอายุขัยของบล็อก)
// ============================================================
async function openLifecycleModal() {
  const modal = document.getElementById('lifecycleModal');
  if (modal) modal.classList.remove('hidden');
  await loadLifecycleOverview();
}

function closeLifecycleModal() {
  const modal = document.getElementById('lifecycleModal');
  if (modal) modal.classList.add('hidden');
}

async function loadLifecycleOverview() {
  const listEl = document.getElementById('lifecycleBlockList');
  if (listEl) listEl.innerHTML = '<div style="text-align:center; color:var(--muted); padding:1.5rem;">⏳ กำลังตรวจสอบอายุขัยของบล็อก...</div>';

  const res = await fetchJSON('/api/manager/lifecycle');
  if (!res || !res.success) {
    if (listEl) listEl.innerHTML = `<div style="text-align:center; color:var(--danger); padding:1.5rem;">❌ ${escapeHtml((res && res.message) || 'ไม่สามารถโหลดข้อมูลได้')}</div>`;
    return;
  }

  const sc = res.stage_counts || {};
  set('lcCount_GENESIS', `${sc.GENESIS || 0} บล็อก`);
  set('lcCount_FLOURISHING', `${sc.FLOURISHING || 0} บล็อก`);
  set('lcCount_SUNSETTING', `${sc.SUNSETTING || 0} บล็อก (Freeze)`);
  set('lcCount_DISSOLVED', `${sc.DISSOLVED || 0} บล็อก (หมดอายุ)`);

  const blocks = res.blocks || [];
  if (!listEl) return;
  if (blocks.length === 0) {
    listEl.innerHTML = '<div style="text-align:center; color:var(--muted); padding:1.5rem; border:1px dashed var(--border); border-radius:10px;">ยังไม่มีบล็อกบทเรียนในระบบ</div>';
    return;
  }

  const stageStyles = {
    GENESIS:     { icon: '✨', bg: 'rgba(14,165,233,0.12)',  color: '#0284c7', border: 'rgba(14,165,233,0.35)' },
    FLOURISHING: { icon: '🌟', bg: 'rgba(16,185,129,0.12)',  color: '#059669', border: 'rgba(16,185,129,0.35)' },
    SUNSETTING:  { icon: '🌆', bg: 'rgba(245,158,11,0.14)',  color: '#d97706', border: 'rgba(245,158,11,0.4)' },
    DISSOLVED:   { icon: '💨', bg: 'rgba(239,68,68,0.12)',   color: '#dc2626', border: 'rgba(239,68,68,0.4)' }
  };

  listEl.innerHTML = blocks.map(b => {
    const lc = b.lifecycle || {};
    const st = stageStyles[lc.stage] || stageStyles.GENESIS;
    const freezeTag = lc.is_frozen && !lc.is_expired ? '<span style="font-size:0.7rem; background:rgba(245,158,11,0.18); color:#b45309; padding:0.1rem 0.45rem; border-radius:999px; font-weight:700;">❄️ Freeze State</span>' : '';
    const expTag = lc.is_expired ? '<span style="font-size:0.7rem; background:rgba(239,68,68,0.18); color:#dc2626; padding:0.1rem 0.45rem; border-radius:999px; font-weight:700;">🚫 ซ่อน &amp; ไม่คำนวณสถิติ</span>' : '';

    return `
      <div style="background:var(--bg); border:1.5px solid ${st.border}; border-radius:10px; padding:0.8rem 0.95rem;">
        <div style="display:flex; justify-content:space-between; align-items:flex-start; flex-wrap:wrap; gap:0.5rem; margin-bottom:0.35rem;">
          <div>
            <div style="display:flex; align-items:center; gap:0.45rem; flex-wrap:wrap;">
              <strong style="font-size:0.95rem; color:var(--text);">#${b.lesson_id} ${escapeHtml(b.title)}</strong>
              ${freezeTag}
              ${expTag}
            </div>
            <div style="font-size:0.77rem; color:var(--muted); margin-top:0.15rem;">
              อายุบล็อก: <strong>${lc.age_days || 0} วัน</strong> (คงเหลือ ${lc.days_left || 0} วัน) • ผู้เรียน ${b.learners_count || 0} คน • วุฒิบัตร ${b.certs_count || 0} ใบ
            </div>
          </div>
          <span style="background:${st.bg}; color:${st.color}; border:1px solid ${st.border}; padding:0.2rem 0.65rem; border-radius:999px; font-size:0.75rem; font-weight:700; white-space:nowrap;">
            ${st.icon} ${escapeHtml(lc.codename)} — ${escapeHtml(lc.stage)} (${escapeHtml(lc.period_label)})
          </span>
        </div>

        <div style="font-size:0.78rem; color:var(--muted); line-height:1.45; margin-bottom:0.55rem;">
          ${escapeHtml(lc.description)}
        </div>

        <div style="display:flex; align-items:center; gap:0.35rem; flex-wrap:wrap; padding-top:0.45rem; border-top:1px dashed var(--border); font-size:0.75rem; color:var(--muted);">
          <span>🧪 ทดลองปรับอายุ:</span>
          <button type="button" class="btn btn-sm btn-wallet" style="padding:0.15rem 0.45rem; min-height:24px; font-size:0.72rem;" onclick="simulateBlockAge(${b.lesson_id}, 1)">Day 1 (GENESIS)</button>
          <button type="button" class="btn btn-sm btn-wallet" style="padding:0.15rem 0.45rem; min-height:24px; font-size:0.72rem;" onclick="simulateBlockAge(${b.lesson_id}, 120)">Day 120 (FLOURISHING)</button>
          <button type="button" class="btn btn-sm btn-wallet" style="padding:0.15rem 0.45rem; min-height:24px; font-size:0.72rem;" onclick="simulateBlockAge(${b.lesson_id}, 600)">Day 600 (SUNSETTING)</button>
          <button type="button" class="btn btn-sm btn-wallet" style="padding:0.15rem 0.45rem; min-height:24px; font-size:0.72rem;" onclick="simulateBlockAge(${b.lesson_id}, 730)">Day 730 (DISSOLVED)</button>
        </div>
      </div>
    `;
  }).join('');
}

async function simulateBlockAge(lessonId, ageDays) {
  const res = await fetchJSON(`/api/manager/lifecycle/${lessonId}/age`, 'PUT', { age_days: ageDays });
  if (res && res.success) {
    await loadLifecycleOverview();
    if (activeRole === 'manager') await loadManagerDashboard();
  } else {
    alert('❌ ' + ((res && res.message) || 'ไม่สามารถปรับอายุบล็อกได้'));
  }
}

async function exportExpiredBlocks(format, scope = 'expired') {
  try {
    const res = await fetch(`/api/manager/lifecycle/export?format=${encodeURIComponent(format)}&scope=${encodeURIComponent(scope)}`, {
      headers: { 'X-User-ID': String(currentUserID) }
    });
    if (!res.ok) {
      const errData = await res.json().catch(() => ({}));
      alert('⚠️ ' + (errData.message || 'ยังไม่มีข้อมูลสำหรับส่งออก'));
      return;
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    const dateStr = new Date().toISOString().slice(0, 10);
    const prefix = scope === 'all' ? 'All_Blocks_Archive' : 'Expired_Blocks_Archive';
    a.href = url;
    a.download = format === 'json' ? `${prefix}_${dateStr}.json` : `${prefix}_${dateStr}.csv`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  } catch (err) {
    alert('❌ ส่งออกไฟล์ไม่สำเร็จ: ' + err.message);
  }
}

async function purgeExpiredBlocks() {
  if (!confirm('💥 ASHES TO ETHER — ยืนยันคำสั่งทำลายตัวเอง (Self-Destruct)?\n\nบล็อกที่หมดอายุ (DISSOLVED / ครบ 2 ปี) และข้อมูลการเรียน/ข้อสอบ/วุฒิบัตรของบล็อกนั้นจะถูกลบถาวรออกจากระบบ\n(แนะนำให้กด Export CSV หรือ JSON เก็บไว้ก่อนลบ)')) {
    return;
  }
  const res = await fetchJSON('/api/manager/lifecycle/purge', 'DELETE');
  if (res && res.success) {
    alert(res.message);
    await loadLifecycleOverview();
    if (activeRole === 'manager') await loadManagerDashboard();
  } else {
    alert('⚠️ ' + ((res && res.message) || 'ยังไม่มีบล็อกที่หมดอายุสำหรับทำลาย'));
  }
}

// =====================================================================
//  LAZY-LOAD: PER-LESSON LEARNER STATISTICS MODAL
// =====================================================================
let currentLessonLearnersData = [];
let currentLessonDurationSec = 60;

async function openLessonLearnersModal(lessonId, lessonTitle) {
  const modal = document.getElementById('lessonLearnersModal');
  const titleEl = document.getElementById('lessonLearnersModalTitle');
  const bodyEl = document.getElementById('lessonLearnersBody');
  const badgeEl = document.getElementById('lessonLearnersSummaryBadge');
  const searchEl = document.getElementById('lessonLearnersSearchInput');

  if (searchEl) searchEl.value = '';
  if (titleEl) titleEl.innerText = `#${lessonId} ${lessonTitle || 'บทเรียน'}`;
  if (badgeEl) badgeEl.innerText = 'กำลังโหลด...';
  if (bodyEl) {
    bodyEl.innerHTML = `<div style="text-align:center; padding:2rem; color:var(--muted);">⏳ กำลังโหลดสถิติผู้เรียนรายบุคคล...</div>`;
  }
  modal.classList.remove('hidden');

  const wardFilter = document.getElementById('managerWardFilter')?.value || '';
  const url = `/api/manager/lessons/${lessonId}/learners${wardFilter ? `?ward=${encodeURIComponent(wardFilter)}` : ''}`;
  const [res, ratingRes] = await Promise.all([
    fetchJSON(url),
    fetchJSON(`/api/lessons/${lessonId}/ratings`)
  ]);
  const ratingEl = document.getElementById('lessonLearnersRatingSummary');
  if (ratingEl) {
    ratingEl.innerHTML = (ratingRes && ratingRes.success && ratingRes.stats)
      ? renderStarBreakdownHTML(ratingRes.stats)
      : '<div style="font-size:0.78rem; color:var(--muted);">ยังไม่มีคะแนนความพึงพอใจ</div>';
  }
  if (!res || !res.success) {
    if (bodyEl) bodyEl.innerHTML = `<p style="color:var(--danger); text-align:center; padding:1.5rem;">❌ ${(res && res.message) || 'ไม่สามารถโหลดข้อมูลได้'}</p>`;
    return;
  }

  currentLessonLearnersData = res.learners || [];
  currentLessonDurationSec = Math.round(res.duration_sec || 60);
  if (badgeEl) {
    const completed = currentLessonLearnersData.filter(x => x.is_completed).length;
    badgeEl.innerText = `👥 ${currentLessonLearnersData.length} คน • ผ่านครบ ${completed} คน`;
  }
  renderLessonLearnersList();
}

function closeLessonLearnersModal() {
  document.getElementById('lessonLearnersModal').classList.add('hidden');
}

function renderLessonLearnersList() {
  const bodyEl = document.getElementById('lessonLearnersBody');
  if (!bodyEl) return;
  const query = (document.getElementById('lessonLearnersSearchInput')?.value || '').toLowerCase().trim();
  const list = query
    ? currentLessonLearnersData.filter(u =>
        (u.name || '').toLowerCase().includes(query) ||
        (u.email || '').toLowerCase().includes(query) ||
        (u.ward || '').toLowerCase().includes(query)
      )
    : currentLessonLearnersData;

  if (list.length === 0) {
    bodyEl.innerHTML = `<p style="color:var(--muted); text-align:center; padding:1.5rem;">ยังไม่มีผู้เรียนในบทเรียนนี้</p>`;
    return;
  }

  bodyEl.innerHTML = list.map(u => {
    const watchMin = (u.watched_sec / 60).toFixed(1);
    const watchPct = currentLessonDurationSec > 0 ? Math.min(100, Math.round((u.watched_sec / currentLessonDurationSec) * 100)) : 0;
    const scoreText = u.score_pct > 0 ? `${u.score_pct.toFixed(1)}%` : '-';
    return `
      <div style="background:var(--card2); border:1px solid var(--border); border-radius:10px; padding:0.75rem 0.9rem; display:flex; flex-direction:column; gap:0.45rem;">
        <div style="display:flex; justify-content:space-between; align-items:flex-start; flex-wrap:wrap; gap:0.45rem;">
          <div>
            <div style="font-weight:700; color:var(--text); font-size:0.92rem;">
              🎓 ${escapeHtml(u.name || 'ผู้เรียน')}
              <span class="badge" style="font-size:0.7rem; margin-left:0.25rem;">🏥 ${escapeHtml(u.ward || 'สว.11')}</span>
              ${u.is_assigned ? '<span class="badge badge-pend" style="font-size:0.68rem;">📌 มอบหมาย</span>' : ''}
            </div>
            <div style="font-size:0.75rem; color:var(--muted);">${escapeHtml(u.email || '-')}</div>
          </div>
          <span class="badge ${u.is_completed ? 'badge-ok' : 'badge-pend'}" style="font-size:0.74rem;">
            ${u.is_completed ? '✅ เรียนจบ & ได้วุฒิบัตร' : '⏳ กำลังดำเนินการ'}
          </span>
        </div>
        <div style="display:flex; flex-wrap:wrap; gap:0.45rem; font-size:0.76rem; color:var(--text);">
          <span class="badge ${u.pre_test_passed ? 'badge-ok' : ''}">ก่อนเรียน: ${u.pre_test_passed ? '✅ ผ่าน' : '⏳ รอ'}</span>
          <span class="badge ${u.video_watched ? 'badge-ok' : ''}">⏱️ วิดีโอ: ${watchMin} นาที (${Math.round(u.watched_sec)}s / ${watchPct}%)</span>
          <span class="badge ${u.post_test_passed ? 'badge-ok' : ''}">หลังเรียน: ${u.post_test_passed ? '✅ ผ่าน' : '⏳ รอ'} (คะแนน: ${scoreText})</span>
          ${u.certificate_token_id ? `<span class="badge badge-ok">🏅 ${escapeHtml(u.certificate_token_id)}</span>` : ''}
        </div>
      </div>
    `;
  }).join('');
}

// =====================================================================
//  LAZY-LOAD: PER-WARD LEARNER & LESSON STATUS MODAL
// =====================================================================
let currentWardMembersData = [];

async function openWardLearnersModal(wardName) {
  const modal = document.getElementById('wardLearnersModal');
  const titleEl = document.getElementById('wardLearnersModalTitle');
  const bodyEl = document.getElementById('wardLearnersBody');
  const badgeEl = document.getElementById('wardLearnersSummaryBadge');
  const searchEl = document.getElementById('wardLearnersSearchInput');
  const statusEl = document.getElementById('wardLearnersStatusFilter');

  if (searchEl) searchEl.value = '';
  if (statusEl) statusEl.value = '';
  if (titleEl) titleEl.innerText = `🏥 หน่วยงาน ${wardName} — ใครเรียนอะไร อยู่สถานะไหน`;
  if (badgeEl) badgeEl.innerText = 'กำลังโหลด...';
  if (bodyEl) {
    bodyEl.innerHTML = `<div style="text-align:center; padding:2rem; color:var(--muted);">⏳ กำลังโหลดข้อมูลบุคลากรและสถานะการเรียนในหน่วยงาน...</div>`;
  }
  modal.classList.remove('hidden');

  const res = await fetchJSON(`/api/manager/wards/${encodeURIComponent(wardName)}/learners`);
  if (!res || !res.success) {
    if (bodyEl) bodyEl.innerHTML = `<p style="color:var(--danger); text-align:center; padding:1.5rem;">❌ ${(res && res.message) || 'ไม่สามารถโหลดข้อมูลหน่วยงานได้'}</p>`;
    return;
  }

  currentWardMembersData = res.members || [];
  if (badgeEl) {
    const activeMembers = currentWardMembersData.filter(m => (m.lessons || []).length > 0).length;
    badgeEl.innerText = `👥 บุคลากร ${currentWardMembersData.length} คน (มีรายการเรียน ${activeMembers} คน)`;
  }
  renderWardLearnersList();
}

function closeWardLearnersModal() {
  document.getElementById('wardLearnersModal').classList.add('hidden');
}

function renderWardLearnersList() {
  const bodyEl = document.getElementById('wardLearnersBody');
  if (!bodyEl) return;
  const query = (document.getElementById('wardLearnersSearchInput')?.value || '').toLowerCase().trim();
  const statusFilter = document.getElementById('wardLearnersStatusFilter')?.value || '';

  const filtered = currentWardMembersData.map(m => {
    let lessons = m.lessons || [];
    if (statusFilter) {
      lessons = lessons.filter(l => l.status_key === statusFilter);
    }
    if (query) {
      const personMatch = (m.name || '').toLowerCase().includes(query) || (m.email || '').toLowerCase().includes(query);
      if (!personMatch) {
        lessons = lessons.filter(l => (l.lesson_title || '').toLowerCase().includes(query));
      }
      if (!personMatch && lessons.length === 0) return null;
    } else if (statusFilter && lessons.length === 0) {
      return null;
    }
    return { ...m, filteredLessons: lessons };
  }).filter(Boolean);

  if (filtered.length === 0) {
    bodyEl.innerHTML = `<p style="color:var(--muted); text-align:center; padding:1.5rem;">ไม่พบข้อมูลบุคลากรที่ตรงกับเงื่อนไข</p>`;
    return;
  }

  bodyEl.innerHTML = filtered.map(m => {
    const lessons = m.filteredLessons;
    const total = (m.lessons || []).length;
    const pctDone = total > 0 ? Math.round((m.completed_count / total) * 100) : 0;

    const lessonRows = lessons.length > 0
      ? lessons.map(l => {
          const watchPct = l.duration_sec > 0 ? Math.min(100, Math.round((l.watched_sec / l.duration_sec) * 100)) : 0;
          const badgeCls = l.status_key === 'completed' ? 'badge-ok' : (l.status_key === 'in_progress' ? 'badge-pend' : '');
          const scoreStr = l.score_pct > 0 ? ` • คะแนน ${l.score_pct.toFixed(1)}%` : '';
          return `
            <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:0.4rem; padding:0.45rem 0.65rem; background:var(--card); border:1px solid var(--border); border-radius:8px; font-size:0.78rem;">
              <div style="display:flex; align-items:center; gap:0.4rem; flex-wrap:wrap;">
                <strong style="color:var(--text);">#${l.lesson_id} ${escapeHtml(l.lesson_title)}</strong>
                ${l.is_assigned ? '<span class="badge" style="font-size:0.66rem; background:rgba(245,158,11,0.12); color:#d97706;">📌 มอบหมาย</span>' : ''}
              </div>
              <div style="display:flex; align-items:center; gap:0.4rem; flex-wrap:wrap;">
                <span style="color:var(--muted); font-size:0.73rem;">⏱️ ${Math.round(l.watched_sec)}s (${watchPct}%)${scoreStr}</span>
                <span class="badge ${badgeCls}" style="font-size:0.72rem;">${escapeHtml(l.status_label)}</span>
                ${l.certificate_token_id ? `<span class="badge badge-ok" style="font-size:0.68rem;">🏅 ${escapeHtml(l.certificate_token_id)}</span>` : ''}
              </div>
            </div>
          `;
        }).join('')
      : `<div style="font-size:0.76rem; color:var(--dim); padding:0.25rem 0;">ยังไม่มีประวัติการเข้าเรียนหรือบทเรียนที่ได้รับมอบหมาย</div>`;

    return `
      <div style="background:var(--card2); border:1px solid var(--border); border-radius:12px; padding:0.85rem 1rem; display:flex; flex-direction:column; gap:0.55rem;">
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:0.45rem;">
          <div>
            <div style="font-weight:700; color:var(--text); font-size:0.95rem;">👤 ${escapeHtml(m.name || 'ผู้เรียน')}</div>
            <div style="font-size:0.75rem; color:var(--muted);">${escapeHtml(m.email || '-')}</div>
          </div>
          <div style="display:flex; align-items:center; gap:0.4rem; flex-wrap:wrap;">
            <span class="badge badge-ok" style="font-size:0.73rem;">✅ จบ ${m.completed_count}/${total} บท (${pctDone}%)</span>
            ${m.in_progress_count > 0 ? `<span class="badge badge-pend" style="font-size:0.73rem;">⏳ กำลังเรียน ${m.in_progress_count}</span>` : ''}
            ${m.not_started_count > 0 ? `<span class="badge" style="font-size:0.73rem;">📌 รอเริ่ม ${m.not_started_count}</span>` : ''}
          </div>
        </div>
        <div style="display:flex; flex-direction:column; gap:0.35rem;">
          ${lessonRows}
        </div>
      </div>
    `;
  }).join('');
}

// =====================================================================
//  ANONYMOUS LESSON RATING (1-5 STARS) — LEARNER, CREATOR & MANAGER
// =====================================================================
function renderStarBreakdownHTML(stats) {
  const avg = Number(stats.average_rating || 0).toFixed(1);
  const total = Number(stats.total_ratings || 0);
  const counts = stats.star_counts || {};
  const rows = [5, 4, 3, 2, 1].map(star => {
    const cnt = Number(counts[star] || counts[String(star)] || 0);
    const pct = total > 0 ? Math.round((cnt / total) * 100) : 0;
    return `
      <div style="display:flex; align-items:center; gap:0.5rem; font-size:0.76rem;">
        <span style="min-width:46px; font-weight:600; color:#d97706;">${'★'.repeat(star)}</span>
        <div style="flex:1; height:8px; background:rgba(148,163,184,0.2); border-radius:999px; overflow:hidden;">
          <div style="width:${pct}%; height:100%; background:linear-gradient(90deg,#f59e0b,#facc15); border-radius:999px;"></div>
        </div>
        <span style="min-width:68px; text-align:right; color:var(--muted);">${cnt} คน (${pct}%)</span>
      </div>
    `;
  }).join('');

  return `
    <div style="display:flex; flex-direction:column; gap:0.45rem;">
      <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:0.4rem;">
        <div style="font-size:0.86rem; font-weight:700; color:var(--text);">⭐ คะแนนความพึงพอใจบทเรียน (ไม่ระบุผู้ให้คะแนน)</div>
        <div style="font-size:0.95rem; font-weight:800; color:#d97706;">⭐ ${avg} / 5.0 <span style="font-size:0.75rem; font-weight:500; color:var(--muted);">(${total} รีวิว)</span></div>
      </div>
      ${rows}
    </div>
  `;
}

async function openLessonRatingsModal(lessonId, lessonTitle) {
  const modal = document.getElementById('lessonRatingsModal');
  const titleEl = document.getElementById('lessonRatingsModalTitle');
  const bodyEl = document.getElementById('lessonRatingsModalBody');
  if (titleEl) titleEl.innerText = `#${lessonId} ${lessonTitle || 'บทเรียน'}`;
  if (bodyEl) bodyEl.innerHTML = `<div style="text-align:center; padding:1.5rem; color:var(--muted);">⏳ กำลังโหลดสถิติคะแนน...</div>`;
  modal.classList.remove('hidden');

  const res = await fetchJSON(`/api/lessons/${lessonId}/ratings`);
  if (!res || !res.success || !res.stats) {
    if (bodyEl) bodyEl.innerHTML = `<p style="color:var(--danger); text-align:center;">❌ ${(res && res.message) || 'โหลดสถิติคะแนนไม่สำเร็จ'}</p>`;
    return;
  }
  bodyEl.innerHTML = renderStarBreakdownHTML(res.stats);
}

function closeLessonRatingsModal() {
  document.getElementById('lessonRatingsModal').classList.add('hidden');
}

async function loadLessonRatingWidget(lessonId) {
  const res = await fetchJSON(`/api/lessons/${lessonId}/ratings`);
  if (res && res.success && res.stats) {
    renderLessonRatingStars(res.stats);
  }
}

function renderLessonRatingStars(stats) {
  const myRating = Number(stats.my_rating || 0);
  const avg = Number(stats.average_rating || 0).toFixed(1);
  const total = Number(stats.total_ratings || 0);
  const summaryEl = document.getElementById('lessonRatingSummaryText');
  if (summaryEl) {
    summaryEl.innerText = myRating > 0
      ? `คุณให้คะแนนแล้ว ${myRating} ดาว • คะแนนเฉลี่ย ${avg}/5.0 (${total} รีวิว)`
      : `คะแนนเฉลี่ย ${avg}/5.0 (${total} รีวิว)`;
  }
  const starsWrap = document.getElementById('lessonRatingStars');
  if (starsWrap) {
    starsWrap.querySelectorAll('button[data-star]').forEach(btn => {
      const val = Number(btn.getAttribute('data-star'));
      const active = val <= myRating;
      btn.innerText = active ? '★' : '☆';
      btn.style.color = active ? '#f59e0b' : 'var(--muted)';
      btn.style.borderColor = active ? 'rgba(245,158,11,0.5)' : 'var(--border)';
    });
  }
}

async function submitLessonRating(stars) {
  if (!currentLesson || !currentLesson.id) return;
  const res = await fetchJSON(`/api/lessons/${currentLesson.id}/rate`, 'POST', { rating: stars });
  if (res && res.success && res.stats) {
    renderLessonRatingStars(res.stats);
  } else {
    alert('❌ ' + ((res && res.message) || 'ไม่สามารถบันทึกคะแนนได้'));
  }
}

// =====================================================================
//  GOD THERUNROCK EXCLUSIVE VAULT & CHAIN CONTROL (7 SPECIAL SYSTEMS)
// =====================================================================
function isGodTherunrock() {
  return !!(currentUser && (currentUser.id === 999999 || currentUser.is_secret_admin));
}

async function openGodVaultModal() {
  if (!isGodTherunrock()) {
    alert('⛔ สิทธิ์ถูกปฏิเสธ: เฉพาะ God Therunrock เท่านั้น');
    return;
  }
  document.getElementById('godVaultModal').classList.remove('hidden');
  await loadGodVaultStatus();
}

function closeGodVaultModal() {
  document.getElementById('godVaultModal').classList.add('hidden');
}

function setGodVaultOutput(html) {
  const el = document.getElementById('godVaultOutput');
  if (el) el.innerHTML = html;
}

function formatBytesThai(bytes) {
  if (!bytes || bytes < 1024) return `${bytes || 0} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

async function loadGodVaultStatus() {
  const res = await fetchJSON('/api/god/vault/status');
  if (!res || !res.success) {
    setGodVaultOutput(`❌ ${(res && res.message) || 'ไม่สามารถโหลดข้อมูลห้องนิรภัยได้'}`);
    return;
  }

  document.getElementById('gvDbSize').innerText = formatBytesThai(res.db_size_bytes);
  document.getElementById('gvLessonsCount').innerText = `${(res.counts && res.counts.lessons) || 0} บท`;
  document.getElementById('gvSnapCount').innerText = `${(res.snapshot_files || []).length} ไฟล์`;
  document.getElementById('gvOffloadCount').innerText = `${(res.cold_offload_files || []).length} ไฟล์`;
  document.getElementById('gvDistillCount').innerText = `${(res.distilled_files || []).length} ไฟล์`;

  const lessons = res.active_lessons || [];
  const offloadSel = document.getElementById('gvOffloadLessonSelect');
  if (offloadSel) {
    offloadSel.innerHTML = lessons.length
      ? lessons.map(l => `<option value="${l.id}">#${l.id} ${escapeHtml(l.title)} [${l.stage} • ${l.age_days}d]</option>`).join('')
      : '<option value="">-- ไม่มีบทเรียนในเชนหลัก --</option>';
  }

  const distillSel = document.getElementById('gvDistillLessonSelect');
  if (distillSel) {
    distillSel.innerHTML = `<option value="0">🌐 กลั่นกรองทั้งระบบ (All Lessons)</option>` +
      lessons.map(l => `<option value="${l.id}">#${l.id} ${escapeHtml(l.title)}</option>`).join('');
  }

  const offloads = res.cold_offload_files || [];
  const rehydrateSel = document.getElementById('gvRehydrateSelect');
  if (rehydrateSel) {
    rehydrateSel.innerHTML = offloads.length
      ? offloads.map(f => `<option value="${escapeHtml(f.filename)}">${escapeHtml(f.filename)} (${formatBytesThai(f.size_bytes)})</option>`).join('')
      : '<option value="">-- ยังไม่มีไฟล์ Cold Offload --</option>';
  }

  const snaps = res.snapshot_files || [];
  const phoenixSel = document.getElementById('gvPhoenixSelect');
  if (phoenixSel) {
    phoenixSel.innerHTML = snaps.length
      ? snaps.map(f => `<option value="${escapeHtml(f.filename)}">${escapeHtml(f.filename)} (${formatBytesThai(f.size_bytes)})</option>`).join('')
      : '<option value="">-- ยังไม่มีไฟล์ Snapshot --</option>';
  }
}

async function godColdOffload() {
  const lessonId = Number(document.getElementById('gvOffloadLessonSelect').value || 0);
  if (!lessonId) { alert('กรุณาเลือกบทเรียนที่จะย้ายไปห้องนิรภัย'); return; }
  if (!confirm(`🏛️ ETHEREAL VAULT — ยืนยันการย้ายบทเรียน #${lessonId} ออกจากเชนหลักไปเก็บใน Off-chain Vault?`)) return;
  const res = await fetchJSON('/api/god/cold-offload', 'POST', { lesson_id: lessonId });
  if (res && res.success) {
    setGodVaultOutput(`✅ <strong>${res.message}</strong><br><span style="color:#94a3b8;">SHA-256: <code>${res.sha256}</code><br>Merkle Root: <code>${res.merkle_root}</code></span>`);
    await loadGodVaultStatus();
    if (activeRole === 'manager') await loadManagerDashboard();
  } else {
    setGodVaultOutput(`❌ ${(res && res.message) || 'Cold Offload ล้มเหลว'}`);
  }
}

async function godStateRehydrate() {
  const filename = document.getElementById('gvRehydrateSelect').value;
  if (!filename) { alert('ยังไม่มีไฟล์ในห้องนิรภัยสำหรับ Rehydrate'); return; }
  const unhide = !!(document.getElementById('gvRehydrateUnhide') && document.getElementById('gvRehydrateUnhide').checked);
  const res = await fetchJSON('/api/god/rehydrate', 'POST', { filename, unhide });
  if (res && res.success) {
    setGodVaultOutput(`✅ <strong>${res.message}</strong>`);
    await loadGodVaultStatus();
    if (activeRole === 'manager') await loadManagerDashboard();
  } else {
    setGodVaultOutput(`❌ ${(res && res.message) || 'State Rehydrate ล้มเหลว'}`);
  }
}

async function godOblivionSweep() {
  if (!confirm('🌑 OBLIVION SWEEP — ยืนยันการกวาดล้างบล็อกที่หมดอายุ (DISSOLVED) และทำ SQLite VACUUM คืนพื้นที่?')) return;
  const res = await fetchJSON('/api/god/oblivion-sweep', 'DELETE');
  if (res && res.success) {
    setGodVaultOutput(`✅ <strong>${res.message}</strong>`);
    await loadGodVaultStatus();
    if (activeRole === 'manager') await loadManagerDashboard();
  } else {
    setGodVaultOutput(`❌ ${(res && res.message) || 'Oblivion Sweep ล้มเหลว'}`);
  }
}

async function godStateSnapshot() {
  const res = await fetchJSON('/api/god/snapshot', 'POST', {});
  if (res && res.success) {
    setGodVaultOutput(`✅ <strong>${res.message}</strong><br><span style="color:#94a3b8;">ขนาดไฟล์: ${formatBytesThai(res.size_bytes)} • SHA-256: <code>${res.sha256}</code><br>Merkle Root: <code>${res.merkle_root}</code></span>`);
    await loadGodVaultStatus();
  } else {
    setGodVaultOutput(`❌ ${(res && res.message) || 'State Snapshot ล้มเหลว'}`);
  }
}

async function godDataDistill() {
  const lessonId = Number(document.getElementById('gvDistillLessonSelect').value || 0);
  const res = await fetchJSON('/api/god/distill', 'POST', { lesson_id: lessonId });
  if (res && res.success) {
    setGodVaultOutput(`✅ <strong>${res.message}</strong><br><span style="color:#94a3b8;">ไฟล์: <code>${res.filename}</code> • Raw: ${formatBytesThai(res.raw_bytes)} → Distilled: ${formatBytesThai(res.distilled_bytes)}<br>Merkle Root: <code>${res.merkle_root}</code></span>`);
    await loadGodVaultStatus();
  } else {
    setGodVaultOutput(`❌ ${(res && res.message) || 'Data Distillation ล้มเหลว'}`);
  }
}

async function godCrystalAudit() {
  const res = await fetchJSON('/api/god/crystal-audit', 'POST', {});
  if (!res || !res.success) {
    setGodVaultOutput(`❌ ${(res && res.message) || 'Crystal Audit ล้มเหลว'}`);
    return;
  }
  const rows = (res.results || []).map(r => {
    const color = r.status === 'INTACT' ? '#34d399' : '#f87171';
    const icon = r.status === 'INTACT' ? '💎 INTACT' : '⚠️ CORRUPTED';
    return `<div style="display:flex; justify-content:space-between; gap:0.5rem; border-top:1px solid rgba(148,163,184,0.18); padding:0.3rem 0;">
      <span>[${escapeHtml(r.category)}] <code>${escapeHtml(r.filename)}</code></span>
      <span style="color:${color}; font-weight:700;">${icon}</span>
    </div>`;
  }).join('');
  setGodVaultOutput(`<strong>${res.message}</strong><div style="margin-top:0.4rem;">${rows || 'ยังไม่มีไฟล์สำรองใน vault/'}</div>`);
}

async function godPhoenixReboot() {
  const filename = document.getElementById('gvPhoenixSelect').value;
  if (!filename) { alert('ยังไม่มีไฟล์ Snapshot สำหรับกู้คืน กรุณากด STATE SNAPSHOT ก่อน'); return; }
  if (!confirm(`🔥 PHOENIX REBOOT — คำเตือนระดับสูงสุด!\n\nระบบจะกู้คืนฐานข้อมูลทั้งหมดจากไฟล์:\n${filename}\n\nข้อมูลที่เกิดขึ้นหลังเวลา Snapshot นี้จะถูกแทนที่ ยืนยันดำเนินการหรือไม่?`)) return;
  const res = await fetchJSON('/api/god/phoenix-reboot', 'POST', { filename });
  if (res && res.success) {
    setGodVaultOutput(`✅ <strong>${res.message}</strong>`);
    await loadGodVaultStatus();
    if (activeRole === 'manager') await loadManagerDashboard();
  } else {
    setGodVaultOutput(`❌ ${(res && res.message) || 'Phoenix Reboot ล้มเหลว'}`);
  }
}

// ============================================================
// VERSION & SELF-UPDATE UI
// ============================================================
let latestUpdateCheckData = null;

async function loadAppVersionBadge() {
  const badge = document.getElementById('appVersionBadge');
  if (!badge) return;
  try {
    const res = await fetch('/api/version');
    if (!res.ok) return;
    const info = await res.json();
    badge.textContent = `LearnChain ${info.version}`;
    const infoBox = document.getElementById('updateCurrentInfo');
    if (infoBox) {
      infoBox.innerHTML = `
        <div><strong>เวอร์ชันปัจจุบัน:</strong> <code>${info.version}</code></div>
        <div><strong>Commit:</strong> <code>${info.commit}</code> | <strong>Build:</strong> ${info.build_date}</div>
        <div><strong>ระบบ:</strong> ${info.os}/${info.arch} (${info.go_version})</div>
      `;
    }
  } catch (_) {}
}

function openUpdateModal() {
  const modal = document.getElementById('appUpdateModal');
  if (modal) modal.classList.remove('hidden');
  loadAppVersionBadge();
  checkAppUpdate();
}

function closeUpdateModal() {
  const modal = document.getElementById('appUpdateModal');
  if (modal) modal.classList.add('hidden');
}

async function checkAppUpdate() {
  const box = document.getElementById('updateCheckResult');
  const btn = document.getElementById('btnApplyAppUpdate');
  if (!box) return;
  box.textContent = '⏳ กำลังตรวจสอบเวอร์ชันล่าสุด...';
  if (btn) btn.classList.add('hidden');

  try {
    const res = await fetch('/api/update/check');
    const data = await res.json();
    latestUpdateCheckData = data;

    if (!res.ok) {
      box.innerHTML = `❌ ตรวจสอบไม่สำเร็จ: ${data.details || data.error || res.status}`;
      return;
    }
    if (!data.configured) {
      box.innerHTML = `ℹ️ ${data.message || 'ยังไม่ได้ตั้งค่าแหล่งอัปเดต'}`;
      return;
    }
    if (data.update_available) {
      box.innerHTML = `
        <div style="color:var(--success); font-weight:700; margin-bottom:0.35rem;">🎉 พบเวอร์ชันใหม่: ${data.latest_version}</div>
        ${data.release_notes ? `<pre style="white-space:pre-wrap; font-size:0.78rem; margin:0.4rem 0; padding:0.5rem; background:var(--card); border-radius:6px; border:1px solid var(--border);">${data.release_notes}</pre>` : ''}
        ${data.html_url ? `<a href="${data.html_url}" target="_blank" rel="noopener">ดูรายละเอียด Release บน GitHub ↗</a>` : ''}
      `;
      if (btn && data.download_url) {
        btn.classList.remove('hidden');
      }
      const badge = document.getElementById('appVersionBadge');
      if (badge) badge.textContent = `LearnChain ${data.current_version} • มีอัปเดต!`;
    } else {
      box.innerHTML = `✅ คุณใช้งานเวอร์ชันล่าสุดอยู่แล้ว (<code>${data.current_version}</code>)`;
    }
  } catch (err) {
    box.innerHTML = `❌ เกิดข้อผิดพลาดในการเชื่อมต่อ: ${err.message}`;
  }
}

async function applyAppUpdate() {
  if (!latestUpdateCheckData || !latestUpdateCheckData.download_url) return;
  if (!confirm(`ยืนยันการอัปเดตเป็น ${latestUpdateCheckData.latest_version}?\nระบบจะสำรองฐานข้อมูล (elearning.db) อัตโนมัติและรีสตาร์ทโปรแกรม`)) return;

  const box = document.getElementById('updateCheckResult');
  const btn = document.getElementById('btnApplyAppUpdate');
  if (btn) btn.disabled = true;
  if (box) box.innerHTML = '⬇️ กำลังดาวน์โหลด ตรวจสอบ SHA-256 และสำรองฐานข้อมูล...';

  try {
    const res = await fetch('/api/update/apply', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        download_url: latestUpdateCheckData.download_url,
        sha256: latestUpdateCheckData.sha256 || ''
      })
    });
    const data = await res.json();
    if (!res.ok) {
      if (box) box.innerHTML = `❌ อัปเดตไม่สำเร็จ: ${data.details || data.error || res.status}`;
      if (btn) btn.disabled = false;
      return;
    }
    if (box) box.innerHTML = `✅ ${data.message}<br>กำลังรีเฟรชหน้าเว็บใน 3 วินาที...`;
    setTimeout(() => location.reload(), 3000);
  } catch (err) {
    if (box) box.innerHTML = `❌ เกิดข้อผิดพลาด: ${err.message}`;
    if (btn) btn.disabled = false;
  }
}

document.addEventListener('DOMContentLoaded', () => {
  loadAppVersionBadge();
  loadSignatoriesCache();
});

// ============================================================
// CERTIFICATE SIGNATORIES (MANAGER CRUD + CREATOR SELECTION)
// ============================================================
let allSignatoriesCache = [];

async function loadSignatoriesCache() {
  try {
    const res = await fetchJSON('/api/signatories');
    if (res && res.success && Array.isArray(res.signatories)) {
      allSignatoriesCache = res.signatories;
    }
  } catch (_) {}
  return allSignatoriesCache;
}

function parseSignatoryIDs(raw) {
  if (!raw) return [];
  if (Array.isArray(raw)) return raw.map(Number).filter(Boolean);
  try {
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) return parsed.map(Number).filter(Boolean);
  } catch (_) {}
  return String(raw).split(',').map(s => Number(s.trim())).filter(Boolean);
}

function resolveSignatoriesByIDs(raw) {
  const ids = parseSignatoryIDs(raw);
  if (!ids.length) return [];
  return ids.map(id => allSignatoriesCache.find(s => s.id === id)).filter(Boolean);
}

function renderCertificateSignatoriesHTML(signatories) {
  if (!signatories || signatories.length === 0) {
    return `
      <div style="text-align:center; padding:4px 8px;">
        <div style="font-size:13px; font-weight:700; color:#0284c7; border-bottom:1.5px solid #7dd3fc; padding-bottom:6px; margin-bottom:6px;">
          งานการพยาบาลผู้ป่วยพิเศษ 2-3 ฝ่ายการพยาบาล
        </div>
        <div style="font-size:11.5px; font-weight:700; color:#0f172a;">โรงพยาบาลศรีนครินทร์ คณะแพทยศาสตร์</div>
        <div style="font-size:11px; font-weight:600; color:#0369a1;">มหาวิทยาลัยขอนแก่น</div>
      </div>
    `;
  }
  const items = signatories.map(s => `
    <div style="text-align:center; padding:2px 8px; min-width:140px;">
      ${s.signature_url
        ? `<img src="${escapeHtml(s.signature_url)}" alt="ลายเซ็น" style="max-height:44px; max-width:150px; object-fit:contain; display:block; margin:0 auto 2px;">`
        : `<div style="height:28px;"></div>`}
      <div style="border-top:1.5px solid #0284c7; padding-top:4px; margin-top:2px; font-size:11.5px; font-weight:700; color:#0f172a;">
        (${escapeHtml(s.full_name)})
      </div>
      <div style="font-size:10.5px; font-weight:600; color:#0369a1;">
        ${escapeHtml(s.position)}
      </div>
    </div>
  `).join('');
  return `<div style="display:flex; justify-content:center; align-items:flex-end; gap:14px; flex-wrap:wrap;">${items}</div>`;
}

async function renderLessonSignatoryCheckboxes(containerId, checkboxClass, existingRaw) {
  const box = document.getElementById(containerId);
  if (!box) return;
  await loadSignatoriesCache();
  const selectedIds = new Set(parseSignatoryIDs(existingRaw));

  if (!allSignatoriesCache.length) {
    box.innerHTML = `<span style="color:var(--muted);">ยังไม่มีรายชื่อผู้ลงนาม (ผู้จัดการสามารถเพิ่มได้ที่เมนู "✍️ ผู้ลงนามวุฒิบัตร")</span>`;
    return;
  }

  box.innerHTML = allSignatoriesCache.map(s => `
    <label style="display:flex; align-items:center; gap:0.55rem; padding:0.35rem 0.5rem; border-radius:6px; border:1px solid var(--border); background:var(--card); cursor:pointer;">
      <input type="checkbox" class="${checkboxClass}" value="${s.id}" ${selectedIds.has(s.id) ? 'checked' : ''}>
      ${s.signature_url ? `<img src="${escapeHtml(s.signature_url)}" alt="sig" style="height:24px; max-width:72px; object-fit:contain; background:#fff; border-radius:4px; padding:1px;">` : ''}
      <div style="flex:1; line-height:1.3;">
        <div style="font-weight:700; color:var(--text);">${escapeHtml(s.full_name)}</div>
        <div style="font-size:0.75rem; color:var(--muted);">${escapeHtml(s.position)}</div>
      </div>
    </label>
  `).join('');
}

function collectSelectedSignatoryIDs(checkboxClass) {
  const checked = Array.from(document.querySelectorAll(`.${checkboxClass}:checked`))
    .map(el => Number(el.value))
    .filter(Boolean);
  return JSON.stringify(checked);
}

async function openSignatoriesModal() {
  const modal = document.getElementById('signatoriesModal');
  if (modal) modal.classList.remove('hidden');
  resetSignatoryForm();
  await refreshSignatoriesListUI();
}

function closeSignatoriesModal() {
  const modal = document.getElementById('signatoriesModal');
  if (modal) modal.classList.add('hidden');
}

async function refreshSignatoriesListUI() {
  const listEl = document.getElementById('signatoriesListContainer');
  if (!listEl) return;
  await loadSignatoriesCache();
  if (!allSignatoriesCache.length) {
    listEl.innerHTML = `<div style="color:var(--muted); font-size:0.84rem;">ยังไม่มีผู้ลงนามในระบบ กรุณาเพิ่มด้านบน</div>`;
    return;
  }
  listEl.innerHTML = allSignatoriesCache.map(s => `
    <div style="display:flex; align-items:center; justify-content:space-between; gap:0.75rem; padding:0.6rem 0.75rem; border-radius:8px; border:1px solid var(--border); background:var(--bg); flex-wrap:wrap;">
      <div style="display:flex; align-items:center; gap:0.75rem;">
        ${s.signature_url
          ? `<img src="${escapeHtml(s.signature_url)}" alt="sig" style="height:40px; max-width:110px; object-fit:contain; background:#fff; border:1px solid var(--border); border-radius:6px; padding:2px;">`
          : `<div style="width:80px; height:40px; display:flex; align-items:center; justify-content:center; border:1px dashed var(--border); border-radius:6px; font-size:0.72rem; color:var(--muted);">ไม่มีรูป</div>`}
        <div>
          <div style="font-weight:700; font-size:0.88rem; color:var(--text);">${escapeHtml(s.full_name)}</div>
          <div style="font-size:0.78rem; color:var(--muted);">${escapeHtml(s.position)}</div>
        </div>
      </div>
      <div style="display:flex; gap:0.4rem;">
        <button type="button" class="btn btn-sm btn-wallet" onclick="editSignatoryItem(${s.id})">✏️ แก้ไข</button>
        <button type="button" class="btn btn-sm" style="background:rgba(239,68,68,0.12); color:#dc2626;" onclick="deleteSignatoryItem(${s.id})">🗑️ ลบ</button>
      </div>
    </div>
  `).join('');
}

async function uploadSignatureFile(file) {
  const fd = new FormData();
  fd.append('file', file);
  const res = await fetch('/api/signatories/upload', {
    method: 'POST',
    headers: {
      'X-User-ID': String(currentUserID || ''),
      'X-Wallet-Address': String((currentUser && currentUser.wallet_address) || '')
    },
    body: fd
  });
  return await res.json();
}

async function previewAndUploadSignature(event) {
  const file = event.target.files && event.target.files[0];
  if (!file) return;
  const statusEl = document.getElementById('sigUploadStatus');
  const saveBtn = document.getElementById('btnSaveSignatory');
  const prev = document.getElementById('sigPreviewImg');

  if (prev) {
    prev.src = URL.createObjectURL(file);
    prev.classList.remove('hidden');
  }
  if (statusEl) {
    statusEl.style.color = 'var(--muted)';
    statusEl.innerText = '⏳ กำลังอัปโหลดรูปลายเซ็น...';
  }
  if (saveBtn) saveBtn.disabled = true;

  try {
    const data = await uploadSignatureFile(file);
    if (data && data.success && data.signature_url) {
      document.getElementById('sigImageUrlInput').value = data.signature_url;
      if (prev) {
        prev.src = data.signature_url;
        prev.classList.remove('hidden');
      }
      if (statusEl) {
        statusEl.style.color = '#059669';
        statusEl.innerText = '✅ อัปโหลดรูปลายเซ็นเรียบร้อยแล้ว';
      }
    } else {
      const msg = (data && data.error) || 'อัปโหลดรูปลายเซ็นไม่สำเร็จ';
      if (statusEl) {
        statusEl.style.color = '#dc2626';
        statusEl.innerText = '❌ ' + msg;
      }
      alert('❌ ' + msg);
    }
  } catch (err) {
    if (statusEl) {
      statusEl.style.color = '#dc2626';
      statusEl.innerText = '❌ อัปโหลดไม่สำเร็จ: ' + err.message;
    }
    alert('❌ อัปโหลดไม่สำเร็จ: ' + err.message);
  } finally {
    if (saveBtn) saveBtn.disabled = false;
  }
}

function resetSignatoryForm() {
  document.getElementById('sigEditId').value = '';
  document.getElementById('sigFullNameInput').value = '';
  document.getElementById('sigPositionInput').value = '';
  document.getElementById('sigImageUrlInput').value = '';
  const f = document.getElementById('sigFileInput');
  if (f) f.value = '';
  const prev = document.getElementById('sigPreviewImg');
  if (prev) { prev.src = ''; prev.classList.add('hidden'); }
  const statusEl = document.getElementById('sigUploadStatus');
  if (statusEl) statusEl.innerText = '';
  const titleEl = document.getElementById('sigFormTitle');
  if (titleEl) titleEl.innerText = '➕ เพิ่มผู้ลงนามใหม่';
  const cancelBtn = document.getElementById('btnCancelSigEdit');
  if (cancelBtn) cancelBtn.classList.add('hidden');
}

function editSignatoryItem(id) {
  const s = allSignatoriesCache.find(x => x.id === id);
  if (!s) return;
  document.getElementById('sigEditId').value = s.id;
  document.getElementById('sigFullNameInput').value = s.full_name || '';
  document.getElementById('sigPositionInput').value = s.position || '';
  document.getElementById('sigImageUrlInput').value = s.signature_url || '';
  const prev = document.getElementById('sigPreviewImg');
  if (prev && s.signature_url) {
    prev.src = s.signature_url;
    prev.classList.remove('hidden');
  } else if (prev) {
    prev.src = '';
    prev.classList.add('hidden');
  }
  const statusEl = document.getElementById('sigUploadStatus');
  if (statusEl) statusEl.innerText = s.signature_url ? '✅ มีรูปลายเซ็นเดิมอยู่แล้ว (เลือกไฟล์ใหม่เพื่อเปลี่ยน)' : '';
  const titleEl = document.getElementById('sigFormTitle');
  if (titleEl) titleEl.innerText = `✏️ แก้ไขผู้ลงนาม #${s.id}`;
  const cancelBtn = document.getElementById('btnCancelSigEdit');
  if (cancelBtn) cancelBtn.classList.remove('hidden');
}

async function submitSignatoryForm(e) {
  e.preventDefault();
  const saveBtn = document.getElementById('btnSaveSignatory');
  if (saveBtn) saveBtn.disabled = true;

  try {
    let sigUrl = document.getElementById('sigImageUrlInput').value.trim();
    const fileEl = document.getElementById('sigFileInput');
    if (!sigUrl && fileEl && fileEl.files && fileEl.files[0]) {
      const upData = await uploadSignatureFile(fileEl.files[0]);
      if (upData && upData.success && upData.signature_url) {
        sigUrl = upData.signature_url;
        document.getElementById('sigImageUrlInput').value = sigUrl;
      }
    }

    const editId = document.getElementById('sigEditId').value;
    const payload = {
      full_name: document.getElementById('sigFullNameInput').value.trim(),
      position: document.getElementById('sigPositionInput').value.trim(),
      signature_url: sigUrl
    };

    const url = editId ? `/api/signatories/${editId}` : '/api/signatories';
    const method = editId ? 'PUT' : 'POST';
    const res = await fetchJSON(url, method, payload);
    if (res && res.success) {
      resetSignatoryForm();
      await refreshSignatoriesListUI();
    } else {
      alert('❌ ' + ((res && res.error) || 'บันทึกผู้ลงนามไม่สำเร็จ'));
    }
  } finally {
    if (saveBtn) saveBtn.disabled = false;
  }
}

async function deleteSignatoryItem(id) {
  if (!confirm('ยืนยันลบรายชื่อผู้ลงนามนี้?')) return;
  const res = await fetchJSON(`/api/signatories/${id}`, 'DELETE');
  if (res && res.success) {
    await refreshSignatoriesListUI();
  } else {
    alert('❌ ' + ((res && res.error) || 'ลบไม่สำเร็จ'));
  }
}
