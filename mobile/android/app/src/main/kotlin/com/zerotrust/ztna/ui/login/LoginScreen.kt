package com.zerotrust.ztna.ui.login

import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.zerotrust.ztna.R
import com.zerotrust.ztna.viewmodel.UiState
import com.zerotrust.ztna.viewmodel.ZtnaViewModel

@Composable
fun LoginScreen(viewModel: ZtnaViewModel) {
    val uiState by viewModel.uiState.collectAsState()

    var controllerUrl by remember { mutableStateOf("https://") }
    var tenantSlug by remember { mutableStateOf("") }
    val isLoading = uiState is UiState.Loading || uiState is UiState.AwaitingCallback

    if (uiState is UiState.Error) {
        LaunchedEffect(uiState) {
            // Keep showing the error — dismissError() is called on button tap.
        }
    }

    Scaffold { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(horizontal = 32.dp),
            verticalArrangement = Arrangement.Center,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            // Logo / title
            Text(
                text = stringResource(R.string.app_name),
                fontSize = 28.sp,
                fontWeight = FontWeight.Bold,
                textAlign = TextAlign.Center,
            )
            Spacer(Modifier.height(8.dp))
            Text(
                text = stringResource(R.string.login_subtitle),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
            )
            Spacer(Modifier.height(40.dp))

            // Controller URL
            OutlinedTextField(
                value = controllerUrl,
                onValueChange = { controllerUrl = it },
                label = { Text(stringResource(R.string.label_controller_url)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
                enabled = !isLoading,
            )
            Spacer(Modifier.height(16.dp))

            // Tenant slug
            OutlinedTextField(
                value = tenantSlug,
                onValueChange = { tenantSlug = it },
                label = { Text(stringResource(R.string.label_tenant_slug)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
                enabled = !isLoading,
            )
            Spacer(Modifier.height(32.dp))

            // Error banner
            if (uiState is UiState.Error) {
                Card(
                    colors = CardDefaults.cardColors(
                        containerColor = MaterialTheme.colorScheme.errorContainer
                    ),
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Text(
                        text = (uiState as UiState.Error).message,
                        modifier = Modifier.padding(12.dp),
                        color = MaterialTheme.colorScheme.onErrorContainer,
                        style = MaterialTheme.typography.bodySmall,
                    )
                }
                Spacer(Modifier.height(16.dp))
            }

            // Sign In button
            Button(
                onClick = {
                    if (uiState is UiState.Error) viewModel.dismissError()
                    viewModel.beginLogin(
                        controllerUrl = controllerUrl.trimEnd('/'),
                        tenantSlug = tenantSlug.trim(),
                    )
                },
                modifier = Modifier
                    .fillMaxWidth()
                    .height(52.dp),
                enabled = !isLoading && tenantSlug.isNotBlank() && controllerUrl.isNotBlank(),
            ) {
                if (isLoading) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(20.dp),
                        color = MaterialTheme.colorScheme.onPrimary,
                        strokeWidth = 2.dp,
                    )
                } else {
                    Text(stringResource(R.string.btn_sign_in))
                }
            }

            if (uiState is UiState.AwaitingCallback) {
                Spacer(Modifier.height(16.dp))
                Text(
                    text = stringResource(R.string.awaiting_callback),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                )
            }
        }
    }
}
