package mongodb

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"example.com/backend-golang-assignment/internal/domain"
)

type Repository struct{ coll *mongo.Collection }

func NewRepository(coll *mongo.Collection) *Repository { return &Repository{coll: coll} }

func (r *Repository) EnsureIndexes(ctx context.Context) error {
	_, err := r.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("unique_email"),
	})
	return err
}

type userDoc struct {
	ID           bson.ObjectID `bson:"_id,omitempty"`
	Name         string        `bson:"name"`
	Email        string        `bson:"email"`
	PasswordHash string        `bson:"password_hash"`
	CreatedAt    bson.DateTime `bson:"created_at"`
}

func convert(d userDoc) domain.User {
	return domain.User{ID: d.ID.Hex(), Name: d.Name, Email: d.Email,
		PasswordHash: d.PasswordHash, CreatedAt: d.CreatedAt.Time().UTC()}
}

func normalizeErr(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.ErrNotFound
	}
	if mongo.IsDuplicateKeyError(err) {
		return domain.ErrConflict
	}
	return err
}

func oid(id string) (bson.ObjectID, error) {
	v, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return v, domain.ErrInvalid
	}
	return v, nil
}

func (r *Repository) Create(ctx context.Context, u domain.User) (domain.User, error) {
	d := userDoc{ID: bson.NewObjectID(), Name: u.Name, Email: u.Email,
		PasswordHash: u.PasswordHash, CreatedAt: bson.NewDateTimeFromTime(u.CreatedAt)}
	_, err := r.coll.InsertOne(ctx, d)
	if err != nil {
		return domain.User{}, normalizeErr(err)
	}
	return convert(d), nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (domain.User, error) {
	key, err := oid(id)
	if err != nil {
		return domain.User{}, err
	}
	var d userDoc
	if err = r.coll.FindOne(ctx, bson.D{{Key: "_id", Value: key}}).Decode(&d); err != nil {
		return domain.User{}, normalizeErr(err)
	}
	return convert(d), nil
}

func (r *Repository) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	var d userDoc
	if err := r.coll.FindOne(ctx, bson.D{{Key: "email", Value: email}}).Decode(&d); err != nil {
		return domain.User{}, normalizeErr(err)
	}
	return convert(d), nil
}

func (r *Repository) List(ctx context.Context, limit, offset int64) ([]domain.User, error) {
	opts := options.Find().SetLimit(limit).SetSkip(offset).SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "_id", Value: 1}})
	cur, err := r.coll.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	result := make([]domain.User, 0)
	for cur.Next(ctx) {
		var d userDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		result = append(result, convert(d))
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Repository) Update(ctx context.Context, id string, fields domain.UpdateFields) (domain.User, error) {
	key, err := oid(id)
	if err != nil {
		return domain.User{}, err
	}
	sets := bson.D{}
	if fields.Name != nil {
		sets = append(sets, bson.E{Key: "name", Value: *fields.Name})
	}
	if fields.Email != nil {
		sets = append(sets, bson.E{Key: "email", Value: *fields.Email})
	}
	var d userDoc
	err = r.coll.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: key}},
		bson.D{{Key: "$set", Value: sets}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&d)
	if err != nil {
		return domain.User{}, normalizeErr(err)
	}
	return convert(d), nil
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	key, err := oid(id)
	if err != nil {
		return err
	}
	result, err := r.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: key}})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Repository) Count(ctx context.Context) (int64, error) {
	return r.coll.CountDocuments(ctx, bson.D{})
}
