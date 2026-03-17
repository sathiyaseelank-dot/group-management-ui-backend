package com.zerotrust.ztna.ui.settings

import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.zerotrust.ztna.R
import com.zerotrust.ztna.viewmodel.UiState
import com.zerotrust.ztna.viewmodel.ZtnaViewModel
import java.text.SimpleDateFormat
import java.util.*

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    viewModel: ZtnaViewModel,
    onBack: () -> Unit,
) {
    val uiState by viewModel.uiState.collectAsState()
    val workspace = (uiState as? UiState.LoggedIn)?.workspace ?: return

    val dateFormat = remember { SimpleDateFormat("yyyy-MM-dd HH:mm", Locale.getDefault()) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.settings)) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                            contentDescription = stringResource(R.string.back),
                        )
                    }
                },
            )
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {

            // ── Workspace info ──
            InfoSection(title = stringResource(R.string.section_workspace)) {
                InfoRow(label = stringResource(R.string.label_workspace_name), value = workspace.workspaceName)
                InfoRow(label = "Slug", value = workspace.tenantSlug)
                InfoRow(label = stringResource(R.string.label_trust_domain), value = workspace.trustDomain)
            }

            // ── Account info ──
            InfoSection(title = stringResource(R.string.section_account)) {
                InfoRow(label = stringResource(R.string.label_email), value = workspace.userEmail)
                InfoRow(label = stringResource(R.string.label_role), value = workspace.userRole)
            }

            // ── Session info ──
            InfoSection(title = stringResource(R.string.section_session)) {
                val sessionExpiry = remember(workspace.sessionExpiresAt) {
                    dateFormat.format(Date(workspace.sessionExpiresAt * 1000L))
                }
                InfoRow(label = stringResource(R.string.label_session_expires), value = sessionExpiry)
            }

            // ── Device certificate ──
            InfoSection(title = stringResource(R.string.section_device_cert)) {
                InfoRow(label = stringResource(R.string.label_spiffe_id), value = workspace.spiffeId)
                val certExpiry = remember(workspace.certExpiresAt) {
                    dateFormat.format(Date(workspace.certExpiresAt * 1000L))
                }
                InfoRow(label = stringResource(R.string.label_cert_expires), value = certExpiry)
            }

            Spacer(Modifier.weight(1f))

            // ── Actions ──
            OutlinedButton(
                onClick = { viewModel.syncResources() },
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(stringResource(R.string.btn_force_sync))
            }

            Button(
                onClick = { viewModel.signOut() },
                modifier = Modifier.fillMaxWidth(),
                colors = ButtonDefaults.buttonColors(
                    containerColor = MaterialTheme.colorScheme.error,
                    contentColor = MaterialTheme.colorScheme.onError,
                ),
            ) {
                Text(stringResource(R.string.btn_disconnect))
            }
        }
    }
}

@Composable
private fun InfoSection(title: String, content: @Composable ColumnScope.() -> Unit) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceVariant,
        ),
    ) {
        Column(modifier = Modifier.padding(16.dp)) {
            Text(
                text = title,
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.primary,
            )
            Spacer(Modifier.height(8.dp))
            content()
        }
    }
}

@Composable
private fun InfoRow(label: String, value: String) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 2.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        Text(
            text = label,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.weight(0.45f),
        )
        Text(
            text = value,
            style = MaterialTheme.typography.bodySmall,
            modifier = Modifier.weight(0.55f),
        )
    }
}
