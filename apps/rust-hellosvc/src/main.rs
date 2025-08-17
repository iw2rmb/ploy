use tiny_http::{Server, Response};
fn main() {
    let server = Server::http("0.0.0.0:8080").unwrap();
    for rq in server.incoming_requests() {
        let url = rq.url().to_string();
        if url == "/healthz" { let _ = rq.respond(Response::from_string("ok")); }
        else { let _ = rq.respond(Response::from_string("hello from rust-hellosvc")); }
    }
}
