use std::path::PathBuf;
use std::sync::Arc;

use codex_core::CodexThread;
use codex_protocol::ThreadId;
use codex_protocol::parse_command::ParsedCommand;
use codex_protocol::protocol::Op;
use codex_protocol::protocol::ReviewDecision;
use rmcp::model::ErrorData;
use rmcp::model::RequestId;
use serde::Deserialize;
use serde::Serialize;
use serde_json::Value;
use serde_json::json;
use tracing::error;

/// Conforms to the MCP elicitation request params shape, so it can be used as
/// the `params` field of an `elicitation/create` request.
#[derive(Debug, Deserialize, Serialize)]
pub struct ExecApprovalElicitRequestParams {
    // These fields are required so that `params`
    // conforms to ElicitRequestParams.
    pub message: String,

    #[serde(rename = "requestedSchema")]
    pub requested_schema: Value,

    // These are additional fields the client can use to
    // correlate the request with the codex tool call.
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

// TODO(mbolin): ExecApprovalResponse does not conform to ElicitResult. See:
// - https://github.com/modelcontextprotocol/modelcontextprotocol/blob/f962dc1780fa5eed7fb7c8a0232f1fc83ef220cd/schema/2025-06-18/schema.json#L617-L636
// - https://modelcontextprotocol.io/specification/draft/client/elicitation#protocol-messages
// It should have "action" and "content" fields.
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
            error!("failed to submit ExecApproval: {err}");
        }
    });
}
