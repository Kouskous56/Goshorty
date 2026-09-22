(function () {
    'use strict';

    // Canonical API surface (/api/v1/*); the legacy /api/* aliases remain
    // server-side but the SPA targets the versioned routes.
    const API_BASE = '/api/v1';

    let currentToken = localStorage.getItem('token');
    let currentUser = null;

    function escapeHtml(str) {
        return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
    }

    function truncate(str, max) {
        return str.length > max ? str.substring(0, max) + '...' : str;
    }

    function copyText(text) {
        navigator.clipboard.writeText(text).then(() => {
            showSuccess('Copied to clipboard!');
        }).catch(() => {
            showError('Failed to copy to clipboard');
        });
    }

    function showError(message) {
        const isAuth = document.getElementById('authSection').style.display !== 'none';
        const errorDiv = document.getElementById(isAuth ? 'authErrorMsg' : 'dashErrorMsg');
        errorDiv.textContent = message;
        errorDiv.style.display = 'block';
        setTimeout(() => {
            errorDiv.style.display = 'none';
        }, 5000);
    }

    function showSuccess(message) {
        const isAuth = document.getElementById('authSection').style.display !== 'none';
        const successDiv = document.getElementById(isAuth ? 'authSuccessMsg' : 'dashSuccessMsg');
        successDiv.textContent = message;
        successDiv.style.display = 'block';
        setTimeout(() => {
            successDiv.style.display = 'none';
        }, 5000);
    }

    // Central fetch wrapper: attaches the bearer token and reacts to an expired
    // or invalid session on every protected endpoint (login/register excluded,
    // their 401 means "bad credentials" and is handled by the caller).
    async function apiFetch(path, options) {
        const opts = options || {};
        const headers = new Headers(opts.headers || {});
        if (currentToken) {
            headers.set('Authorization', 'Bearer ' + currentToken);
        }
        const res = await fetch(API_BASE + path, Object.assign({}, opts, { headers }));
        if (res.status === 401 && !path.startsWith('/auth/login') && !path.startsWith('/auth/register')) {
            forceSessionExpired();
        }
        return res;
    }

    function forceSessionExpired() {
        clearLocalSession();
        const auth = document.getElementById('authSection');
        const dash = document.getElementById('dashboardSection');
        if (auth) auth.style.display = 'block';
        if (dash) dash.classList.remove('active');
        const msg = document.getElementById('authErrorMsg');
        if (msg) {
            msg.textContent = 'Session expired, please login again';
            msg.style.display = 'block';
        }
    }

    function clearLocalSession() {
        currentToken = null;
        currentUser = null;
        localStorage.removeItem('token');
        localStorage.removeItem('user');
    }

    // On load, never trust a stored token or the cached user object blindly:
    // validate the session against the server first. A 401 (or network error)
    // clears the stale session; otherwise the fresh user replaces the cache.
    async function restoreSession() {
        try {
            const res = await apiFetch('/auth/me');
            if (!res.ok) {
                clearLocalSession();
                return;
            }
            const data = await res.json();
            currentUser = data.user || null;
            if (!currentUser) {
                clearLocalSession();
                return;
            }
            localStorage.setItem('user', JSON.stringify(currentUser));
            loadDashboard();
        } catch (err) {
            clearLocalSession();
        }
    }

    function toggleForm() {
        const loginForm = document.getElementById('loginForm');
        const registerForm = document.getElementById('registerForm');
        loginForm.style.display = loginForm.style.display === 'none' ? 'block' : 'none';
        registerForm.style.display = registerForm.style.display === 'none' ? 'block' : 'none';
    }

    async function handleLogin() {
        const username = document.getElementById('loginUsername').value;
        const password = document.getElementById('loginPassword').value;

        if (!username || !password) {
            showError('Please fill in all fields');
            return;
        }

        try {
            const response = await apiFetch('/auth/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username, password })
            });

            const data = await response.json();

            if (!response.ok) {
                showError(data.message || 'Login failed');
                return;
            }

            currentToken = data.token;
            currentUser = data.user;
            localStorage.setItem('token', currentToken);
            localStorage.setItem('user', JSON.stringify(currentUser));

            showSuccess('Login successful!');
            setTimeout(() => loadDashboard(), 1500);
        } catch (error) {
            showError('Connection error: ' + error.message);
        }
    }

    async function handleRegister() {
        const username = document.getElementById('regUsername').value;
        const email = document.getElementById('regEmail').value;
        const password = document.getElementById('regPassword').value;

        if (!username || !email || !password) {
            showError('Please fill in all fields');
            return;
        }
        const passwordBytes = new TextEncoder().encode(password).length;
        if (passwordBytes < 12 || passwordBytes > 72) {
            showError('Password must be between 12 and 72 bytes');
            return;
        }

        try {
            const response = await apiFetch('/auth/register', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username, email, password })
            });

            const data = await response.json();

            if (!response.ok) {
                showError(data.message || 'Registration failed');
                return;
            }

            currentToken = data.token;
            currentUser = data.user;
            localStorage.setItem('token', currentToken);
            localStorage.setItem('user', JSON.stringify(currentUser));

            showSuccess('Registration successful! Welcome!');
            setTimeout(() => loadDashboard(), 1500);
        } catch (error) {
            showError('Connection error: ' + error.message);
        }
    }

    function loadDashboard() {
        if (!currentToken) {
            showError('No token found');
            return;
        }
        if (!currentUser) {
            forceSessionExpired();
            return;
        }

        document.getElementById('authSection').style.display = 'none';
        document.getElementById('dashboardSection').classList.add('active');
        document.getElementById('currentUser').textContent = currentUser.username;
        document.getElementById('userRole').textContent = currentUser.role.toUpperCase();

        // Show/hide admin tabs
        const adminTabs = document.querySelectorAll('.admin-only');
        if (currentUser.role === 'admin') {
            adminTabs.forEach(tab => tab.style.display = 'block');
            loadAllUrls();
            loadUsers();
        }

        loadMyUrls();
    }

    async function handleCreateUrl() {
        const url = document.getElementById('originalUrl').value.trim();
        const expiresIn = document.getElementById('expiresIn').value;
        const customCode = document.getElementById('customCode').value.trim();

        if (!url) {
            showError('Please enter a URL');
            return;
        }

        if (!url.startsWith('http://') && !url.startsWith('https://')) {
            showError('URL must start with http:// or https://');
            return;
        }

        try {
            const response = await apiFetch('/urls', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    url,
                    expires_in: expiresIn,
                    custom_code: customCode
                })
            });

            const data = await response.json();

            if (!response.ok) {
                showError(data.message || 'Failed to create URL');
                return;
            }

            // Show result
            document.getElementById('resultCode').textContent = data.short_code;
            document.getElementById('resultUrl').textContent = data.short_url;
            document.getElementById('resultExpires').textContent = new Date(data.expires_at).toLocaleString();
            document.getElementById('resultSection').style.display = 'block';

            // Clear form
            document.getElementById('originalUrl').value = '';
            document.getElementById('customCode').value = '';

            showSuccess('URL shortened successfully!');
            loadMyUrls();
        } catch (error) {
            showError('Error: ' + error.message);
        }
    }

    function copyToClipboard() {
        copyText(document.getElementById('resultUrl').textContent);
    }

    async function loadMyUrls() {
        try {
            const response = await apiFetch('/urls?limit=200');

            if (!response.ok) {
                showError('Failed to load URLs');
                return;
            }

            const data = await response.json();
            const urls = data.urls || [];

            let totalVisits = 0;
            let html = '';

            if (urls.length === 0) {
                html = '<div class="empty-state"><div class="empty-state-icon">📭</div><p>No URLs yet. Create your first short URL!</p></div>';
            } else {
                urls.forEach(url => {
                    totalVisits += url.visits || 0;
                    const expiresAt = new Date(url.expires_at);
                    const isExpired = expiresAt < new Date();

                    const esc = escapeHtml;
                    html += `
                            <div class="url-item" style="${isExpired ? 'opacity: 0.5;' : ''}">
                                <div class="url-item-header">
                                    <span class="url-item-code">${esc(url.short_code)}</span>
                                    <div class="url-item-actions">
                                        ${!isExpired ? `<button class="btn-view" data-action="open" data-code="${esc(url.short_code)}">Open</button>` : ''}
                                        <button class="btn-copy" data-action="copy-original" data-url="${esc(url.original_url)}">Copy Original</button>
                                        <button class="btn-delete" data-action="delete-url" data-code="${esc(url.short_code)}">Delete</button>
                                    </div>
                                </div>
                                <div class="url-item-meta">
                                    <div>Original: <strong>${esc(truncate(url.original_url, 60))}</strong></div>
                                    <div>Visits: <strong>${esc(url.visits)}</strong></div>
                                    <div>Expires: <strong>${esc(expiresAt.toLocaleString())} ${isExpired ? '(EXPIRED)' : ''}</strong></div>
                                </div>
                            </div>
                        `;
                });
            }

            document.getElementById('myUrlsList').innerHTML = html;
            document.getElementById('myTotalUrls').textContent = data.total !== undefined ? data.total : urls.length;
            document.getElementById('myTotalVisits').textContent = totalVisits;
        } catch (error) {
            showError('Error loading URLs: ' + error.message);
        }
    }

    async function loadAllUrls() {
        try {
            const response = await apiFetch('/urls?limit=200');

            if (!response.ok) {
                showError('Failed to load all URLs');
                return;
            }

            const data = await response.json();
            const urls = data.urls || [];

            let totalVisits = 0;
            let html = '';

            if (urls.length === 0) {
                html = '<div class="empty-state"><div class="empty-state-icon">📭</div><p>No URLs in system yet</p></div>';
            } else {
                urls.forEach(url => {
                    totalVisits += url.visits || 0;
                    const expiresAt = new Date(url.expires_at);
                    const isExpired = expiresAt < new Date();

                    const esc = escapeHtml;
                    html += `
                            <div class="url-item" style="${isExpired ? 'opacity: 0.5;' : ''}">
                                <div class="url-item-header">
                                    <span class="url-item-code">${esc(url.short_code)}</span>
                                    <div class="url-item-actions">
                                        <button class="btn-delete" data-action="admin-delete-url" data-code="${esc(url.short_code)}">Delete</button>
                                    </div>
                                </div>
                                <div class="url-item-meta">
                                    <div>URL: <strong>${esc(truncate(url.original_url, 60))}</strong></div>
                                    <div>Visits: <strong>${esc(url.visits)}</strong></div>
                                    <div>Expires: <strong>${esc(expiresAt.toLocaleString())} ${isExpired ? '(EXPIRED)' : ''}</strong></div>
                                </div>
                            </div>
                        `;
                });
            }

            document.getElementById('allUrlsList').innerHTML = html;
            document.getElementById('totalUrls').textContent = data.total !== undefined ? data.total : urls.length;
            document.getElementById('totalVisits').textContent = totalVisits;
        } catch (error) {
            showError('Error loading all URLs: ' + error.message);
        }
    }

    async function loadUsers() {
        try {
            const response = await apiFetch('/auth/users?limit=200');

            if (!response.ok) {
                showError('Failed to load users');
                return;
            }

            const data = await response.json();
            const users = data.users || [];

            const esc = escapeHtml;
            let html = '';
            users.forEach(user => {
                const createdDate = esc(new Date(user.created_at * 1000).toLocaleDateString());
                const role = esc(user.role);
                const roleUpper = esc(user.role.toUpperCase());
                const newRole = user.role === 'admin' ? 'user' : 'admin';
                const roleBadge = `<span class="role-badge role-${role}">${roleUpper}</span>`;
                const username = esc(user.username);
                const email = esc(user.email);

                html += `
                        <tr>
                            <td><strong>${username}</strong></td>
                            <td>${email}</td>
                            <td>${roleBadge}</td>
                            <td>${createdDate}</td>
                            <td>
                                <button class="btn-copy" style="background: #f39c12;" data-action="toggle-role" data-username="${esc(user.username)}" data-new-role="${esc(newRole)}">
                                    Change to ${roleUpper === 'ADMIN' ? 'User' : 'Admin'}
                                </button>
                                ${user.username !== 'admin' ? `<button class="btn-delete" data-action="delete-user" data-username="${esc(user.username)}">Delete</button>` : ''}
                            </td>
                        </tr>
                    `;
            });

            document.getElementById('usersTableBody').innerHTML = html;
        } catch (error) {
            showError('Error loading users: ' + error.message);
        }
    }

    async function deleteUrl(code) {
        if (!confirm('Delete this URL?')) return;

        try {
            const response = await apiFetch('/urls/' + encodeURIComponent(code), {
                method: 'DELETE'
            });

            if (!response.ok) {
                showError('Failed to delete URL');
                return;
            }

            showSuccess('URL deleted');
            loadMyUrls();
        } catch (error) {
            showError('Error: ' + error.message);
        }
    }

    async function adminDeleteUrl(code) {
        if (!confirm('Delete this URL?')) return;

        try {
            const response = await apiFetch('/urls/' + encodeURIComponent(code), {
                method: 'DELETE'
            });

            if (!response.ok) {
                showError('Failed to delete URL');
                return;
            }

            showSuccess('URL deleted');
            loadAllUrls();
        } catch (error) {
            showError('Error: ' + error.message);
        }
    }

    async function toggleUserRole(username, newRole) {
        try {
            const response = await apiFetch('/auth/users/' + encodeURIComponent(username) + '/role', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ role: newRole })
            });

            if (!response.ok) {
                showError('Failed to update user');
                return;
            }

            showSuccess('User role updated');
            loadUsers();
        } catch (error) {
            showError('Error: ' + error.message);
        }
    }

    async function deleteUser(username) {
        if (!confirm(`Delete user "${username}"?`)) return;

        try {
            const response = await apiFetch('/auth/users/' + encodeURIComponent(username), {
                method: 'DELETE'
            });

            if (!response.ok) {
                showError('Failed to delete user');
                return;
            }

            showSuccess('User deleted');
            loadUsers();
        } catch (error) {
            showError('Error: ' + error.message);
        }
    }

    async function handleChangePassword() {
        const currentPassword = document.getElementById('currentPassword').value;
        const newPassword = document.getElementById('newPassword').value;
        const confirmNewPassword = document.getElementById('confirmNewPassword').value;

        if (!currentPassword || !newPassword || !confirmNewPassword) {
            showError('Please fill in all password fields');
            return;
        }
        const passwordBytes = new TextEncoder().encode(newPassword).length;
        if (passwordBytes < 12 || passwordBytes > 72) {
            showError('New password must be between 12 and 72 bytes');
            return;
        }
        if (newPassword !== confirmNewPassword) {
            showError('New passwords do not match');
            return;
        }

        try {
            const response = await apiFetch('/auth/password', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    current_password: currentPassword,
                    new_password: newPassword
                })
            });
            const data = await response.json();
            if (!response.ok) {
                showError(data.message || 'Failed to change password');
                return;
            }
            document.getElementById('currentPassword').value = '';
            document.getElementById('newPassword').value = '';
            document.getElementById('confirmNewPassword').value = '';
            alert('Password changed. All previous sessions were revoked; please sign in again.');
            clearLocalSession();
            location.reload();
        } catch (error) {
            showError('Connection error: ' + error.message);
        }
    }

    async function handleRevokeSessions() {
        if (!confirm('Revoke every session and sign out now?')) {
            return;
        }
        try {
            const response = await apiFetch('/auth/revoke', {
                method: 'POST'
            });
            const data = await response.json();
            if (!response.ok) {
                showError(data.message || 'Failed to revoke sessions');
                return;
            }
            clearLocalSession();
            location.reload();
        } catch (error) {
            showError('Connection error: ' + error.message);
        }
    }

    function switchTab(event, tabName) {
        // Hide all tab contents
        const tabContents = document.querySelectorAll('.tab-content');
        tabContents.forEach(tab => tab.classList.remove('active'));

        // Deactivate all buttons
        const tabButtons = document.querySelectorAll('.tab-button');
        tabButtons.forEach(btn => btn.classList.remove('active'));

        // Show selected tab and activate button
        document.getElementById(tabName).classList.add('active');
        event.target.classList.add('active');
    }

    function handleLogout() {
        if (confirm('Logout?')) {
            clearLocalSession();
            location.reload();
        }
    }

    // ---- Event wiring (no inline handlers) ----

    window.addEventListener('load', async () => {
        if (currentToken) {
            await restoreSession();
        }
    });

    document.addEventListener('DOMContentLoaded', () => {
        document.getElementById('loginBtn').addEventListener('click', handleLogin);
        document.getElementById('registerBtn').addEventListener('click', handleRegister);
        document.getElementById('logoutBtn').addEventListener('click', handleLogout);
        document.getElementById('createUrlBtn').addEventListener('click', handleCreateUrl);
        document.getElementById('copyResultBtn').addEventListener('click', copyToClipboard);
        document.getElementById('changePasswordBtn').addEventListener('click', handleChangePassword);
        document.getElementById('revokeSessionsBtn').addEventListener('click', handleRevokeSessions);

        document.querySelectorAll('.toggle-link').forEach(link => {
            link.addEventListener('click', toggleForm);
        });

        document.querySelectorAll('.tab-button').forEach(btn => {
            btn.addEventListener('click', (event) => switchTab(event, btn.dataset.tab));
        });

        // Enter key submits the active auth form.
        document.getElementById('loginPassword').addEventListener('keydown', (e) => {
            if (e.key === 'Enter') handleLogin();
        });
        document.getElementById('regPassword').addEventListener('keydown', (e) => {
            if (e.key === 'Enter') handleRegister();
        });
    });

    // Delegated clicks for dynamically rendered action buttons.
    document.addEventListener('click', (event) => {
        const btn = event.target.closest('[data-action]');
        if (!btn) return;
        switch (btn.dataset.action) {
            case 'open':
                window.open('/r/' + btn.dataset.code);
                break;
            case 'copy-original':
                copyText(btn.dataset.url);
                break;
            case 'delete-url':
                deleteUrl(btn.dataset.code);
                break;
            case 'admin-delete-url':
                adminDeleteUrl(btn.dataset.code);
                break;
            case 'toggle-role':
                toggleUserRole(btn.dataset.username, btn.dataset.newRole);
                break;
            case 'delete-user':
                deleteUser(btn.dataset.username);
                break;
        }
    });
})();