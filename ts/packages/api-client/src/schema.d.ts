// Generated from api/openapi.yaml — run `pnpm codegen` to regenerate.

export interface paths {
  "/workflows/{id}": {
    post: operations["executeWorkflow"];
  };
  "/workflows/{id}/stream": {
    get: operations["streamWorkflow"];
  };
}

export interface components {
  schemas: {
    WorkflowRequest: {
      workflow_id?: string;
      thread_id?: string;
      input: Record<string, unknown>;
    };
    WorkflowResponse: {
      thread_id?: string;
      output?: Record<string, unknown>;
      error?: string;
    };
    StreamEvent: {
      type: "step_start" | "llm_token" | "step_end" | "error" | "done";
      step_id?: string;
      token?: string;
      output?: unknown;
      error?: string;
    };
    ErrorResponse: string;
  };
  parameters: {
    WorkflowID: string;
  };
}

export interface operations {
  executeWorkflow: {
    parameters: {
      path: { id: string };
    };
    requestBody: {
      content: {
        "application/json": components["schemas"]["WorkflowRequest"];
      };
    };
    responses: {
      200: {
        content: {
          "application/json": components["schemas"]["WorkflowResponse"];
        };
      };
      400: {
        content: {
          "application/json": components["schemas"]["ErrorResponse"];
        };
      };
      404: {
        content: {
          "application/json": components["schemas"]["ErrorResponse"];
        };
      };
      500: {
        content: {
          "application/json": components["schemas"]["WorkflowResponse"];
        };
      };
    };
  };
  streamWorkflow: {
    parameters: {
      path: { id: string };
      query?: { input?: string };
    };
    responses: {
      200: {
        content: {
          "text/event-stream": components["schemas"]["StreamEvent"];
        };
      };
      404: { content: never };
    };
  };
}
