use std::path::PathBuf;
use std::sync::Arc;

use codex_core::CodexThread;
use codex_protocol::ThreadId;
use codex_protocol::parse_command::ParsedCommand;
use codex_protocol::protocol::Op;
use codex_protocol::protocol::ReviewDecision;
use rmcp::model::RequestId;
use serde::Deserialize;
use serde::Serialize;
use serde_json::Value;

#[derive(Debug, Deserialize, Serialize)]
pub struct ExecApprovalElicitRequestParams {
    pub message: String,
    #[serde(rename = "requestedSchema")]
    pub requested_schema: Value,
    #[serde(rename = "threadId")]
    pub thread_id: ThreadId,
    pub codex_elicitation: String,
    pub codex_mcp_tool_call_id: String,
    pub codex_event_id: String,
    pub codex_call_id: String,
    pub codex_command: Vec<String>,
    pub codex_cwd: PathBuf,
    pub codex_parsed_cmd: Vec<ParsedCommand>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ExecApprovalResponse {
    pub decision: ReviewDecision,
}

#[allow(clippy::too_many_arguments)]
pub(crate) async fn handle_exec_approval_request(
    _command: Vec<String>,
    _cwd: PathBuf,
    _outgoing: Arc<crate::outgoing_message::OutgoingMessageSender>,
    codex: Arc<CodexThread>,
    _request_id: RequestId,
    _tool_call_id: String,
    event_id: String,
    _call_id: String,
    approval_id: String,
    _codex_parsed_cmd: Vec<ParsedCommand>,
    _thread_id: ThreadId,
) {
    tokio::spawn(async move {
        if let Err(err) = codex
            .submit(Op::ExecApproval {
                id: approval_id,
                turn_id: Some(event_id),
                decision: ReviewDecision::Approved,
            })
            .await
        {
            tracing::error!("failed to submit ExecApproval: {err}");
        }
    });
}
