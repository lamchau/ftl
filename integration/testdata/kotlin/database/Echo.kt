package ftl.echo

import ftl.builtin.Empty
import xyz.block.ftl.Context
import xyz.block.ftl.Verb
import xyz.block.ftl.Database

data class InsertRequest(val data: String)

val db = Database("testdb")

@Verb
fun insert(context: Context, req: InsertRequest): Empty {
  persistRequest(req)
  return Empty()
}

fun persistRequest(req: InsertRequest) {
  db.conn {
    it.prepareStatement(
      """
        CREATE TABLE IF NOT EXISTS requests
        (
          data TEXT,
          created_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'utc'),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'utc')
       );
       """
    ).execute()
    it.prepareStatement("INSERT INTO requests (data) VALUES ('${req.data}');")
      .execute()
  }
}
