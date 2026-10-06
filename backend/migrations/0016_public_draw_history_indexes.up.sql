-- Bounded public archive reads sort by planned draw time, not ingestion time.
CREATE INDEX periods_brand_draw_archive ON periods(brand_id,draw_at DESC,sequence DESC,id DESC);
CREATE INDEX periods_game_draw_archive ON periods(brand_id,game_id,draw_at DESC,sequence DESC,id DESC);
