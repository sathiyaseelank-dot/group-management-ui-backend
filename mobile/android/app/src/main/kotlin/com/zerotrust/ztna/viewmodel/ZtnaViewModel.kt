package com.zerotrust.ztna.viewmodel

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.zerotrust.ztna.uniffi.WorkspaceState
import com.zerotrust.ztna.uniffi.ZtnaException
import com.zerotrust.ztna.uniffi.beginLogin
import com.zerotrust.ztna.uniffi.completeLogin
import com.zerotrust.ztna.uniffi.disconnect
import com.zerotrust.ztna.uniffi.listWorkspaces
import com.zerotrust.ztna.uniffi.sync
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** The URL to open in a Chrome Custom Tab after beginLogin succeeds. */
data class PendingAuth(val authUrl: String)

sealed class UiState {
    object Loading : UiState()
    object LoggedOut : UiState()
    data class AwaitingCallback(val authUrl: String) : UiState()
    data class LoggedIn(val workspace: WorkspaceState) : UiState()
    data class Error(val message: String) : UiState()
}

class ZtnaViewModel(application: Application) : AndroidViewModel(application) {

    private val _uiState = MutableStateFlow<UiState>(UiState.Loading)
    val uiState: StateFlow<UiState> = _uiState.asStateFlow()

    /** The tenant slug chosen during sign-in — carried forward to completeLogin. */
    private var pendingTenantSlug: String = ""
    private var pendingControllerUrl: String = ""

    private val dataDir: String get() =
        getApplication<android.app.Application>().filesDir.absolutePath

    init {
        loadSavedSession()
    }

    private fun loadSavedSession() {
        viewModelScope.launch {
            val workspaces = withContext(Dispatchers.IO) {
                runCatching { listWorkspaces(dataDir) }.getOrNull()
            }
            _uiState.value = if (!workspaces.isNullOrEmpty()) {
                UiState.LoggedIn(workspaces.first())
            } else {
                UiState.LoggedOut
            }
        }
    }

    /**
     * Called when the user taps "Sign In".
     * Returns the auth URL so the UI can open a Chrome Custom Tab.
     * redirect_uri is now handled server-side; not passed from the client.
     */
    fun beginLogin(controllerUrl: String, tenantSlug: String) {
        pendingTenantSlug = tenantSlug
        pendingControllerUrl = controllerUrl
        _uiState.value = UiState.Loading

        viewModelScope.launch {
            val result = withContext(Dispatchers.IO) {
                runCatching { beginLogin(controllerUrl, tenantSlug, dataDir) }
            }
            result.fold(
                onSuccess = { authUrl -> _uiState.value = UiState.AwaitingCallback(authUrl) },
                onFailure = { e -> _uiState.value = UiState.Error(e.message ?: "Login failed") }
            )
        }
    }

    /**
     * Called by MainActivity when the deep link `ztna://callback?session_code=...` arrives.
     */
    fun completeLogin(sessionCode: String) {
        _uiState.value = UiState.Loading

        viewModelScope.launch {
            val result = withContext(Dispatchers.IO) {
                runCatching {
                    completeLogin(
                        controllerUrl = pendingControllerUrl,
                        sessionCode = sessionCode,
                        dataDir = dataDir,
                    )
                }
            }
            result.fold(
                onSuccess = { ws -> _uiState.value = UiState.LoggedIn(ws) },
                onFailure = { e -> _uiState.value = UiState.Error(e.message ?: "Login failed") }
            )
        }
    }

    fun syncResources() {
        val current = _uiState.value as? UiState.LoggedIn ?: return
        viewModelScope.launch {
            val result = withContext(Dispatchers.IO) {
                runCatching {
                    sync(
                        tenantSlug = current.workspace.tenantSlug,
                        controllerUrl = pendingControllerUrl,
                        dataDir = dataDir,
                    )
                }
            }
            result.fold(
                onSuccess = { ws -> _uiState.value = UiState.LoggedIn(ws) },
                onFailure = { e -> _uiState.value = UiState.Error(e.message ?: "Sync failed") }
            )
        }
    }

    fun signOut() {
        val current = _uiState.value as? UiState.LoggedIn ?: return
        viewModelScope.launch {
            withContext(Dispatchers.IO) {
                runCatching {
                    disconnect(
                        tenantSlug = current.workspace.tenantSlug,
                        controllerUrl = pendingControllerUrl,
                        dataDir = dataDir,
                    )
                }
            }
            _uiState.value = UiState.LoggedOut
        }
    }

    fun dismissError() {
        _uiState.value = UiState.LoggedOut
    }
}
