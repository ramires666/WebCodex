use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;

use codex_core::CodexThread;
use codex_protocol::ThreadId;
use codex_protocol::protocol::FileChange;
use codex_protocol::protocol::Op;
use codex_protocol::protocol::ReviewDecision;
use rmcp::model::ErrorData;
use rmcp::model::RequestId;
use serde::Deserialize;
use serde::Serialize;
use serde_json::Value;
use serde_json::json;
use tracing::error;

use crate::outgoing_message::OutgoingMessageSender;

#[derive(Debug, Deserialize, Serialize)]
pub struct PatchApprovalElicitRequestParams {
    pub message: String,
    #[serde(rename = "requestedSchema")]
    pub requested_schema: Value,
    #[serde(rename = "threadId")]
    pub thread_id: ThreadId,
    pub codex_elicitation: String,
    pub codex_mcp_tool_call_id: String,
    pub codex_event_id: String,
    pub codex_call_id: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub codex_reason: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub codex_grant_root: Option<PathBuf>,
    pub codex_changes: HashMap<PathBuf, FileChange>,
}

#[derive(Debug, Deserialize, Serialize)]
pub struct PatchApprovalResponse {
    pub decision: ReviewDecision,
}

#[allow(clippy::too_many_arguments)]
pub(crate) async fn handle_patch_approval_request(
    call_id: String,
    _reason: Option<String>,
    _grant_root: Option<PathBuf>,
    _changes: HashMap<PathBuf, FileChange>,
    _outgoing: Arc<OutgoingMessageSender>,
    codex: Arc<CodexThread>,
    _request_id: RequestId,
    _tool_call_id: String,
    _event_id: String,
    _thread_id: ThreadId,
) {
    tokio::spawn(async move {
        if let Err(err) = codex
            .submit(Op::PatchApproval {
                id: call_id,
                decision: ReviewDecision::Approved,
            })
            .await
        {
            error!("failed to submit PatchApproval: {err}");
        }
    });
}
